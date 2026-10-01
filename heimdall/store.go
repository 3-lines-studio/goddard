package heimdall

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const MaxValue = 64 * 1024

const tokenColumns = "id, name, owner_kind, owner_id, project, env, keys, role, created_at, expires_at, last_used"

const (
	RoleAgent = "agent"
	RoleAdmin = "admin"
)

type ErrorKind int

const (
	ErrBad ErrorKind = iota
	ErrInternal
)

type Error struct {
	Kind    ErrorKind
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

func bad(message string) error {
	return &Error{Kind: ErrBad, Message: message}
}

func internal(message string) error {
	return &Error{Kind: ErrInternal, Message: message}
}

type Token struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Owner     Owner    `json:"owner"`
	Project   string   `json:"project"`
	Env       string   `json:"env"`
	Keys      []string `json:"keys"`
	Role      string   `json:"role"`
	CreatedAt int64    `json:"created_at"`
	ExpiresAt *int64   `json:"expires_at"`
	LastUsed  *int64   `json:"last_used"`
}

type NewToken struct {
	Name    string
	Owner   Owner
	Project string
	Env     string
	Keys    []string
	Role    string
	TTL     *int64
}

type AuditRow struct {
	At      int64   `json:"at"`
	Actor   string  `json:"actor"`
	Owner   Owner   `json:"owner"`
	Action  string  `json:"action"`
	Project string  `json:"project"`
	Env     string  `json:"env"`
	Key     *string `json:"key"`
}

type Store struct {
	db  *sql.DB
	key Key
}

// NewStore takes the pool the app already has, the same one it migrates and
// the same one its other tables use.
func NewStore(db *sql.DB, key Key) *Store {
	return &Store{db: db, key: key}
}

func (s *Store) Close() error {
	return s.db.Close()
}

// Names is what was declared plus what already has secrets: the environments
// that existed before there was a table still show up, with nothing migrated.
func (s *Store) Names(ctx context.Context, owner Owner) ([]string, error) {
	return s.namesOf(ctx, s.db, owner)
}

func (s *Store) CreateEnvironment(ctx context.Context, owner Owner, project, env, actor string) error {
	if err := owner.check(); err != nil {
		return err
	}
	if err := slug(project); err != nil {
		return err
	}
	if err := slug(env); err != nil {
		return err
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO heimdall.environments (owner_kind, owner_id, project, env, created_at) VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING",
			owner.Kind, owner.ID, project, env, now())
		if err != nil {
			return internal(err.Error())
		}
		return audit(ctx, tx, owner, actor, "env-create", project, env, nil)
	})
}

// DropEnvironment takes the tokens that pointed at the environment with it:
// they make no sense over something that stopped existing.
func (s *Store) DropEnvironment(ctx context.Context, owner Owner, project, env, actor string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		secrets, err := tx.ExecContext(ctx, "DELETE FROM heimdall.secrets WHERE owner_kind = $1 AND owner_id = $2 AND project = $3 AND env = $4", owner.Kind, owner.ID, project, env)
		if err != nil {
			return internal(err.Error())
		}
		declared, err := tx.ExecContext(ctx, "DELETE FROM heimdall.environments WHERE owner_kind = $1 AND owner_id = $2 AND project = $3 AND env = $4", owner.Kind, owner.ID, project, env)
		if err != nil {
			return internal(err.Error())
		}
		tokens, err := tx.ExecContext(ctx, "DELETE FROM heimdall.tokens WHERE owner_kind = $1 AND owner_id = $2 AND project = $3 AND env = $4", owner.Kind, owner.ID, project, env)
		if err != nil {
			return internal(err.Error())
		}
		if count(secrets)+count(declared)+count(tokens) == 0 {
			return bad(fmt.Sprintf("%s/%s no existe", project, env))
		}
		return audit(ctx, tx, owner, actor, "env-drop", project, env, nil)
	})
}

func (s *Store) DropProject(ctx context.Context, owner Owner, project, actor string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		secrets, err := tx.ExecContext(ctx, "DELETE FROM heimdall.secrets WHERE owner_kind = $1 AND owner_id = $2 AND project = $3", owner.Kind, owner.ID, project)
		if err != nil {
			return internal(err.Error())
		}
		declared, err := tx.ExecContext(ctx, "DELETE FROM heimdall.environments WHERE owner_kind = $1 AND owner_id = $2 AND project = $3", owner.Kind, owner.ID, project)
		if err != nil {
			return internal(err.Error())
		}
		tokens, err := tx.ExecContext(ctx, "DELETE FROM heimdall.tokens WHERE owner_kind = $1 AND owner_id = $2 AND project = $3", owner.Kind, owner.ID, project)
		if err != nil {
			return internal(err.Error())
		}
		if count(secrets)+count(declared)+count(tokens) == 0 {
			return bad(fmt.Sprintf("%s no existe", project))
		}
		return audit(ctx, tx, owner, actor, "project-drop", project, "", nil)
	})
}

// RenameEnvironment moves the derived key and the data with it, so every value
// is sealed again: the old box does not open under the new name.
func (s *Store) RenameEnvironment(ctx context.Context, owner Owner, project, env, to, actor string) error {
	if err := slug(to); err != nil {
		return err
	}
	if env == to {
		return nil
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		moved, err := s.secretsOf(ctx, tx, owner, project, env)
		if err != nil {
			return err
		}
		taken, err := s.secretsOf(ctx, tx, owner, project, to)
		if err != nil {
			return err
		}
		if len(taken) > 0 {
			return bad(fmt.Sprintf("%s/%s ya tiene secretos", project, to))
		}
		subkey := s.key.Derive(secretContext(owner, project, to))
		for name, value := range moved {
			sealed, err := subkey.Seal([]byte(value), []byte(aad(owner, project, to, name)))
			if err != nil {
				return internal(err.Error())
			}
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO heimdall.secrets (owner_kind, owner_id, project, env, name, value, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7)",
				owner.Kind, owner.ID, project, to, name, sealed, now(),
			); err != nil {
				return internal(err.Error())
			}
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM heimdall.secrets WHERE owner_kind = $1 AND owner_id = $2 AND project = $3 AND env = $4", owner.Kind, owner.ID, project, env); err != nil {
			return internal(err.Error())
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM heimdall.environments WHERE owner_kind = $1 AND owner_id = $2 AND project = $3 AND env = $4", owner.Kind, owner.ID, project, to); err != nil {
			return internal(err.Error())
		}
		if _, err := tx.ExecContext(ctx, "UPDATE heimdall.environments SET env = $1 WHERE owner_kind = $2 AND owner_id = $3 AND project = $4 AND env = $5", to, owner.Kind, owner.ID, project, env); err != nil {
			return internal(err.Error())
		}
		return audit(ctx, tx, owner, actor, "env-rename", project, to, &env)
	})
}

func (s *Store) RenameProject(ctx context.Context, owner Owner, project, to, actor string) error {
	if err := slug(to); err != nil {
		return err
	}
	if project == to {
		return nil
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		names, err := s.namesOf(ctx, tx, owner)
		if err != nil {
			return err
		}
		for _, name := range names {
			if len(name) > len(to)+1 && name[:len(to)+1] == to+"/" {
				return bad(fmt.Sprintf("%s ya existe", to))
			}
		}
		rows, err := tx.QueryContext(ctx, `SELECT env FROM heimdall.environments WHERE owner_kind = $1 AND owner_id = $2 AND project = $3
             UNION
             SELECT DISTINCT env FROM heimdall.secrets WHERE owner_kind = $1 AND owner_id = $2 AND project = $3`, owner.Kind, owner.ID, project)
		if err != nil {
			return internal(err.Error())
		}
		envs := []string{}
		for rows.Next() {
			var env string
			if err := rows.Scan(&env); err != nil {
				rows.Close()
				return internal(err.Error())
			}
			envs = append(envs, env)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return internal(err.Error())
		}
		if len(envs) == 0 {
			return bad(fmt.Sprintf("%s no existe", project))
		}
		for _, env := range envs {
			held, err := s.secretsOf(ctx, tx, owner, project, env)
			if err != nil {
				return err
			}
			subkey := s.key.Derive(secretContext(owner, to, env))
			for name, value := range held {
				sealed, err := subkey.Seal([]byte(value), []byte(aad(owner, to, env, name)))
				if err != nil {
					return internal(err.Error())
				}
				if _, err := tx.ExecContext(ctx,
					"INSERT INTO heimdall.secrets (owner_kind, owner_id, project, env, name, value, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7)",
					owner.Kind, owner.ID, to, env, name, sealed, now(),
				); err != nil {
					return internal(err.Error())
				}
			}
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM heimdall.secrets WHERE owner_kind = $1 AND owner_id = $2 AND project = $3", owner.Kind, owner.ID, project); err != nil {
			return internal(err.Error())
		}
		if _, err := tx.ExecContext(ctx, "UPDATE heimdall.environments SET project = $1 WHERE owner_kind = $2 AND owner_id = $3 AND project = $4", to, owner.Kind, owner.ID, project); err != nil {
			return internal(err.Error())
		}
		return audit(ctx, tx, owner, actor, "project-rename", to, "", &project)
	})
}

func (s *Store) Secrets(ctx context.Context, owner Owner, project, env string) (map[string]string, error) {
	return s.secretsOf(ctx, s.db, owner, project, env)
}

func (s *Store) secretsOf(ctx context.Context, db querier, owner Owner, project, env string) (map[string]string, error) {
	subkey := s.key.Derive(secretContext(owner, project, env))
	rows, err := db.QueryContext(ctx,
		"SELECT name, value FROM heimdall.secrets WHERE owner_kind = $1 AND owner_id = $2 AND project = $3 AND env = $4", owner.Kind, owner.ID, project, env)
	if err != nil {
		return nil, internal(err.Error())
	}
	defer rows.Close()
	secrets := map[string]string{}
	for rows.Next() {
		var name string
		var value []byte
		if err := rows.Scan(&name, &value); err != nil {
			return nil, internal(err.Error())
		}
		plain, err := subkey.Open(value, []byte(aad(owner, project, env, name)))
		if err != nil {
			return nil, internal(err.Error())
		}
		if !utf8.Valid(plain) {
			return nil, internal(fmt.Sprintf("%s no es texto", name))
		}
		secrets[name] = string(plain)
	}
	return secrets, rows.Err()
}

func (s *Store) Set(ctx context.Context, owner Owner, project, env, name, value, actor string) error {
	if err := owner.check(); err != nil {
		return err
	}
	if err := slug(project); err != nil {
		return err
	}
	if err := slug(env); err != nil {
		return err
	}
	if err := keyName(name); err != nil {
		return err
	}
	if len(value) > MaxValue {
		return bad(fmt.Sprintf("el valor pasa los %d bytes", MaxValue))
	}
	sealed, err := s.key.Derive(secretContext(owner, project, env)).Seal([]byte(value), []byte(aad(owner, project, env, name)))
	if err != nil {
		return internal(err.Error())
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO heimdall.secrets (owner_kind, owner_id, project, env, name, value, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7)
             ON CONFLICT (owner_kind, owner_id, project, env, name) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			owner.Kind, owner.ID, project, env, name, sealed, now(),
		); err != nil {
			return internal(err.Error())
		}
		return audit(ctx, tx, owner, actor, "set", project, env, &name)
	})
}

func (s *Store) Unset(ctx context.Context, owner Owner, project, env, name, actor string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		removed, err := tx.ExecContext(ctx,
			"DELETE FROM heimdall.secrets WHERE owner_kind = $1 AND owner_id = $2 AND project = $3 AND env = $4 AND name = $5", owner.Kind, owner.ID, project, env, name)
		if err != nil {
			return internal(err.Error())
		}
		if count(removed) == 0 {
			return bad(fmt.Sprintf("%s no está en %s/%s", name, project, env))
		}
		return audit(ctx, tx, owner, actor, "unset", project, env, &name)
	})
}

func (s *Store) Tokens(ctx context.Context) ([]Token, error) {
	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf("SELECT %s FROM heimdall.tokens ORDER BY created_at", tokenColumns))
	if err != nil {
		return nil, internal(err.Error())
	}
	defer rows.Close()
	return scanTokens(rows, false)
}

func (s *Store) CreateToken(ctx context.Context, new NewToken, actor string) (Token, string, error) {
	role := new.Role
	if role == "" {
		role = RoleAgent
	}
	if role != RoleAgent && role != RoleAdmin {
		return Token{}, "", bad(fmt.Sprintf("«%s» no es un rol", role))
	}
	if role == RoleAdmin {
		if new.Keys != nil {
			return Token{}, "", bad("un token de administración ve todo")
		}
	} else {
		if err := slugOrAll(new.Project); err != nil {
			return Token{}, "", err
		}
		if err := slugOrAll(new.Env); err != nil {
			return Token{}, "", err
		}
	}
	if strings.TrimSpace(new.Name) == "" {
		return Token{}, "", bad("el token necesita un nombre")
	}
	if err := new.Owner.check(); err != nil {
		return Token{}, "", err
	}
	plain, err := RandomHex(24)
	if err != nil {
		return Token{}, "", internal(err.Error())
	}
	id, err := RandomHex(6)
	if err != nil {
		return Token{}, "", internal(err.Error())
	}
	token := Token{
		ID:        id,
		Name:      new.Name,
		Owner:     new.Owner,
		Project:   new.Project,
		Env:       new.Env,
		Keys:      new.Keys,
		Role:      role,
		CreatedAt: now(),
	}
	if new.TTL != nil {
		expires := now() + *new.TTL
		token.ExpiresAt = &expires
	}
	var keys any
	if token.Keys != nil {
		encoded, err := json.Marshal(token.Keys)
		if err != nil {
			return Token{}, "", internal(err.Error())
		}
		keys = encoded
	}
	plain = "hd_" + plain
	err = s.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO heimdall.tokens (id, name, owner_kind, owner_id, project, env, keys, hash, role, created_at, expires_at, last_used)
             VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NULL)`,
			token.ID, token.Name, token.Owner.Kind, token.Owner.ID, token.Project, token.Env, keys, Hash(plain), token.Role, token.CreatedAt, token.ExpiresAt,
		); err != nil {
			return internal(err.Error())
		}
		return audit(ctx, tx, token.Owner, actor, "token-create", token.Project, token.Env, &token.Name)
	})
	if err != nil {
		return Token{}, "", err
	}
	return token, plain, nil
}

func (s *Store) Revoke(ctx context.Context, id, actor string) (Token, error) {
	tokens, err := s.Tokens(ctx)
	if err != nil {
		return Token{}, err
	}
	var found *Token
	for index := range tokens {
		if tokens[index].ID == id {
			found = &tokens[index]
			break
		}
	}
	if found == nil {
		return Token{}, bad(fmt.Sprintf("no hay token %s", id))
	}
	token := *found
	err = s.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM heimdall.tokens WHERE id = $1", id); err != nil {
			return internal(err.Error())
		}
		return audit(ctx, tx, token.Owner, actor, "token-revoke", token.Project, token.Env, &token.Name)
	})
	if err != nil {
		return Token{}, err
	}
	return token, nil
}

func (s *Store) Find(ctx context.Context, plain string) (*Token, error) {
	presented := Hash(plain)
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf("SELECT %s, hash FROM heimdall.tokens", tokenColumns))
	if err != nil {
		return nil, internal(err.Error())
	}
	defer rows.Close()
	for rows.Next() {
		token, hash, err := scanToken(rows, true)
		if err != nil {
			return nil, internal(err.Error())
		}
		if !Equal(hash, presented) {
			continue
		}
		if token.ExpiresAt != nil && *token.ExpiresAt <= now() {
			return nil, bad("ese token venció")
		}
		return &token, nil
	}
	return nil, rows.Err()
}

func (s *Store) Touch(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE heimdall.tokens SET last_used = $1 WHERE id = $2", now(), id); err != nil {
		return internal(err.Error())
	}
	return nil
}

func (s *Store) Audit(ctx context.Context, owner Owner, actor, action, project, env string, name *string) error {
	return audit(ctx, s.db, owner, actor, action, project, env, name)
}

func (s *Store) AuditLog(ctx context.Context, limit int) ([]AuditRow, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT at, actor, owner_kind, owner_id, action, project, env, name FROM heimdall.audit ORDER BY seq DESC LIMIT $1", limit)
	if err != nil {
		return nil, internal(err.Error())
	}
	defer rows.Close()
	log := []AuditRow{}
	for rows.Next() {
		var row AuditRow
		if err := rows.Scan(&row.At, &row.Actor, &row.Owner.Kind, &row.Owner.ID, &row.Action, &row.Project, &row.Env, &row.Key); err != nil {
			return nil, internal(err.Error())
		}
		log = append(log, row)
	}
	return log, rows.Err()
}

func (s *Store) namesOf(ctx context.Context, db querier, owner Owner) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT project || '/' || env FROM heimdall.environments
         WHERE owner_kind = $1 AND owner_id = $2
         UNION
         SELECT DISTINCT project || '/' || env FROM heimdall.secrets
         WHERE owner_kind = $1 AND owner_id = $2
         ORDER BY 1`, owner.Kind, owner.ID)
	if err != nil {
		return nil, internal(err.Error())
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, internal(err.Error())
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// write runs the whole operation in one transaction: what the store reads
// inside it sees the same state it is about to change, and a failure in the
// middle leaves nothing half done.
func (s *Store) write(ctx context.Context, work func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return internal(err.Error())
	}
	defer tx.Rollback()
	if err := work(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return internal(err.Error())
	}
	return nil
}

type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func audit(ctx context.Context, db querier, owner Owner, actor, action, project, env string, name *string) error {
	if _, err := db.ExecContext(ctx,
		"INSERT INTO heimdall.audit (at, actor, owner_kind, owner_id, action, project, env, name) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
		now(), actor, owner.Kind, owner.ID, action, project, env, name,
	); err != nil {
		return internal(err.Error())
	}
	return nil
}

func scanTokens(rows *sql.Rows, withHash bool) ([]Token, error) {
	tokens := []Token{}
	for rows.Next() {
		token, _, err := scanToken(rows, withHash)
		if err != nil {
			return nil, internal(err.Error())
		}
		tokens = append(tokens, token)
	}
	return tokens, rows.Err()
}

func scanToken(row interface{ Scan(...any) error }, withHash bool) (Token, string, error) {
	var token Token
	var keys []byte
	var hash string
	targets := []any{&token.ID, &token.Name, &token.Owner.Kind, &token.Owner.ID, &token.Project, &token.Env, &keys, &token.Role,
		&token.CreatedAt, &token.ExpiresAt, &token.LastUsed}
	if withHash {
		targets = append(targets, &hash)
	}
	if err := row.Scan(targets...); err != nil {
		return Token{}, "", err
	}
	if len(keys) > 0 {
		if err := json.Unmarshal(keys, &token.Keys); err != nil {
			return Token{}, "", err
		}
	}
	return token, hash, nil
}

func count(result sql.Result) int64 {
	rows, err := result.RowsAffected()
	if err != nil {
		return 0
	}
	return rows
}

func secretContext(owner Owner, project, env string) string {
	return fmt.Sprintf("secrets/%s/%s/%s", owner.String(), project, env)
}

// The sealed box carries its own name inside, so moving it to another row does
// not turn it into another secret.
func aad(owner Owner, project, env, name string) string {
	return fmt.Sprintf("secrets/%s/%s/%s/%s", owner.String(), project, env, name)
}

func now() int64 {
	return time.Now().Unix()
}

func slugOrAll(name string) error {
	if name == "*" {
		return nil
	}
	return slug(name)
}

func slug(name string) error {
	if name == "" || len(name) > 64 {
		return bad(fmt.Sprintf("«%s» tiene que medir entre 1 y 64", name))
	}
	for _, char := range name {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-' || char == '_' {
			continue
		}
		return bad(fmt.Sprintf("«%s» sólo acepta minúsculas, números, guiones y guiones bajos", name))
	}
	return nil
}

func keyName(name string) error {
	if name == "" {
		return bad("la clave no puede estar vacía")
	}
	for index, char := range name {
		first := char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char == '_'
		rest := first || char >= '0' && char <= '9'
		if index == 0 && !first || index > 0 && !rest {
			return bad(fmt.Sprintf("«%s» no es un nombre de variable válido", name))
		}
	}
	return nil
}
