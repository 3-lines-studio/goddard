package heimdall

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Imported is what came across. There is no login or session count on purpose:
// they expire in minutes and days, they are not worth carrying.
type Imported struct {
	Secrets      int
	Environments int
	Tokens       int
	Audit        int
}

// ImportSQLite copies the store the Rust crate left in its SQLite file into
// this one, once. It opens every sealed box with the derivation that store
// used and seals it again with this one, which carries the owner: the master
// key is the same, the row is not. The token hashes and the audit rows travel
// as they are. Everything goes in one transaction.
func ImportSQLite(ctx context.Context, path string, into *Store, owner Owner) (Imported, error) {
	if err := owner.check(); err != nil {
		return Imported{}, err
	}
	source, err := sql.Open("sqlite", path)
	if err != nil {
		return Imported{}, fmt.Errorf("no pude abrir %q: %v", path, err)
	}
	defer source.Close()
	if err := source.PingContext(ctx); err != nil {
		return Imported{}, fmt.Errorf("no pude abrir %q: %v", path, err)
	}
	imported := Imported{}
	err = into.write(ctx, func(tx *sql.Tx) error {
		if imported.Environments, err = copyEnvironments(ctx, source, tx, owner); err != nil {
			return err
		}
		if imported.Secrets, err = copySecrets(ctx, source, tx, into.key, owner); err != nil {
			return err
		}
		if imported.Tokens, err = copyTokens(ctx, source, tx, owner); err != nil {
			return err
		}
		if imported.Audit, err = copyAudit(ctx, source, tx, owner); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return Imported{}, err
	}
	return imported, nil
}

func copyEnvironments(ctx context.Context, source *sql.DB, tx *sql.Tx, owner Owner) (int, error) {
	rows, err := source.QueryContext(ctx, "SELECT project, env, created_at FROM environments")
	if err != nil {
		return 0, internal(err.Error())
	}
	defer rows.Close()
	copied := 0
	for rows.Next() {
		var project, env string
		var createdAt int64
		if err := rows.Scan(&project, &env, &createdAt); err != nil {
			return 0, internal(err.Error())
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO heimdall.environments (owner_kind, owner_id, project, env, created_at) VALUES ($1, $2, $3, $4, $5)",
			owner.Kind, owner.ID, project, env, createdAt,
		); err != nil {
			return 0, internal(err.Error())
		}
		copied++
	}
	return copied, rows.Err()
}

func copySecrets(ctx context.Context, source *sql.DB, tx *sql.Tx, key Key, owner Owner) (int, error) {
	rows, err := source.QueryContext(ctx, "SELECT project, env, name, value, updated_at FROM secrets")
	if err != nil {
		return 0, internal(err.Error())
	}
	defer rows.Close()
	copied := 0
	for rows.Next() {
		var project, env, name string
		var value []byte
		var updatedAt int64
		if err := rows.Scan(&project, &env, &name, &value, &updatedAt); err != nil {
			return 0, internal(err.Error())
		}
		plain, err := key.Derive(legacySecretContext(project, env)).Open(value, []byte(legacyAAD(project, env, name)))
		if err != nil {
			return 0, internal(err.Error())
		}
		sealed, err := key.Derive(secretContext(owner, project, env)).Seal(plain, []byte(aad(owner, project, env, name)))
		if err != nil {
			return 0, internal(err.Error())
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO heimdall.secrets (owner_kind, owner_id, project, env, name, value, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7)",
			owner.Kind, owner.ID, project, env, name, sealed, updatedAt,
		); err != nil {
			return 0, internal(err.Error())
		}
		copied++
	}
	return copied, rows.Err()
}

func legacySecretContext(project, env string) string {
	return fmt.Sprintf("secrets/%s/%s", project, env)
}

func legacyAAD(project, env, name string) string {
	return fmt.Sprintf("secrets/%s/%s/%s", project, env, name)
}

func copyTokens(ctx context.Context, source *sql.DB, tx *sql.Tx, owner Owner) (int, error) {
	rows, err := source.QueryContext(ctx,
		"SELECT id, name, project, env, keys, hash, admin, created_at, expires_at, last_used FROM tokens")
	if err != nil {
		return 0, internal(err.Error())
	}
	defer rows.Close()
	copied := 0
	for rows.Next() {
		var id, name, project, env, hash string
		var keys *string
		var admin int64
		var createdAt int64
		var expiresAt, lastUsed *int64
		if err := rows.Scan(&id, &name, &project, &env, &keys, &hash, &admin, &createdAt, &expiresAt, &lastUsed); err != nil {
			return 0, internal(err.Error())
		}
		role := RoleAgent
		if admin != 0 {
			role = RoleAdmin
		}
		var keyList any
		if keys != nil {
			keyList = []byte(*keys)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO heimdall.tokens (id, name, owner_kind, owner_id, project, env, keys, hash, role, created_at, expires_at, last_used)
             VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			id, name, owner.Kind, owner.ID, project, env, keyList, hash, role, createdAt, expiresAt, lastUsed,
		); err != nil {
			return 0, internal(err.Error())
		}
		copied++
	}
	return copied, rows.Err()
}

func copyAudit(ctx context.Context, source *sql.DB, tx *sql.Tx, owner Owner) (int, error) {
	rows, err := source.QueryContext(ctx,
		"SELECT at, actor, action, project, env, name FROM audit ORDER BY rowid")
	if err != nil {
		return 0, internal(err.Error())
	}
	defer rows.Close()
	copied := 0
	for rows.Next() {
		var at int64
		var actor, action, project, env string
		var name *string
		if err := rows.Scan(&at, &actor, &action, &project, &env, &name); err != nil {
			return 0, internal(err.Error())
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO heimdall.audit (at, actor, owner_kind, owner_id, action, project, env, name) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
			at, actor, owner.Kind, owner.ID, action, project, env, name,
		); err != nil {
			return 0, internal(err.Error())
		}
		copied++
	}
	return copied, rows.Err()
}
