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
// this one, once. It does not decrypt anything: the sealed boxes, the token
// hashes and the audit rows travel as they are, so the same master key keeps
// opening them. Everything goes in one transaction.
func ImportSQLite(ctx context.Context, path string, into *Store) (Imported, error) {
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
		if imported.Environments, err = copyEnvironments(ctx, source, tx); err != nil {
			return err
		}
		if imported.Secrets, err = copySecrets(ctx, source, tx); err != nil {
			return err
		}
		if imported.Tokens, err = copyTokens(ctx, source, tx); err != nil {
			return err
		}
		if imported.Audit, err = copyAudit(ctx, source, tx); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return Imported{}, err
	}
	return imported, nil
}

func copyEnvironments(ctx context.Context, source *sql.DB, tx *sql.Tx) (int, error) {
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
			"INSERT INTO heimdall.environments (project, env, created_at) VALUES ($1, $2, $3)",
			project, env, createdAt,
		); err != nil {
			return 0, internal(err.Error())
		}
		copied++
	}
	return copied, rows.Err()
}

func copySecrets(ctx context.Context, source *sql.DB, tx *sql.Tx) (int, error) {
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
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO heimdall.secrets (project, env, name, value, updated_at) VALUES ($1, $2, $3, $4, $5)",
			project, env, name, value, updatedAt,
		); err != nil {
			return 0, internal(err.Error())
		}
		copied++
	}
	return copied, rows.Err()
}

func copyTokens(ctx context.Context, source *sql.DB, tx *sql.Tx) (int, error) {
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
			`INSERT INTO heimdall.tokens (id, name, project, env, keys, hash, role, created_at, expires_at, last_used)
             VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			id, name, project, env, keyList, hash, role, createdAt, expiresAt, lastUsed,
		); err != nil {
			return 0, internal(err.Error())
		}
		copied++
	}
	return copied, rows.Err()
}

func copyAudit(ctx context.Context, source *sql.DB, tx *sql.Tx) (int, error) {
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
			"INSERT INTO heimdall.audit (at, actor, action, project, env, name) VALUES ($1, $2, $3, $4, $5, $6)",
			at, actor, action, project, env, name,
		); err != nil {
			return 0, internal(err.Error())
		}
		copied++
	}
	return copied, rows.Err()
}
