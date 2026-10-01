package heimdall

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const MaxValue = 64 * 1024

const tokenColumns = "id, name, project, env, keys, role, created_at, expires_at, last_used"

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
	Project string
	Env     string
	Keys    []string
	Role    string
	TTL     *int64
}

type AuditRow struct {
	At      int64   `json:"at"`
	Actor   string  `json:"actor"`
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
func (s *Store) Names(ctx context.Context) ([]string, error) {
	return s.namesOf(ctx, s.db)
}

func (s *Store) CreateEnvironment(ctx context.Context, project, env, actor string) error {
	if err := slug(project); err != nil {
		return err
	}
	if err := slug(env); err != nil {
		return err
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO heimdall.environments (project, env, created_at) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING",
			project, env, now())
		if err != nil {
			return internal(err.Error())
		}
		return audit(ctx, tx, actor, "env-create", project, env, nil)
	})
}

// DropEnvironment takes the tokens that pointed at the environment with it:
// they make no sense over something that stopped existing.
func (s *Store) DropEnvironment(ctx context.Context, project, env, actor string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		secrets, err := tx.ExecContext(ctx, "DELETE FROM heimdall.secrets WHERE project = $1 AND env = $2", project, env)
		if err != nil {
			return internal(err.Error())
		}
		declared, err := tx.ExecContext(ctx, "DELETE FROM heimdall.environments WHERE project = $1 AND env = $2", project, env)
		if err != nil {
			return internal(err.Error())
		}
		tokens, err := tx.ExecContext(ctx, "DELETE FROM heimdall.tokens WHERE project = $1 AND env = $2", project, env)
		if err != nil {
			return internal(err.Error())
		}
		if count(secrets)+count(declared)+count(tokens) == 0 {
			return bad(fmt.Sprintf("%s/%s no existe", project, env))
		}
		return audit(ctx, tx, actor, "env-drop", project, env, nil)
	})
}

func (s *Store) DropProject(ctx context.Context, project, actor string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		secrets, err := tx.ExecContext(ctx, "DELETE FROM heimdall.secrets WHERE project = $1", project)
		if err != nil {
			return internal(err.Error())
		}
		declared, err := tx.ExecContext(ctx, "DELETE FROM heimdall.environments WHERE project = $1", project)
		if err != nil {
			return internal(err.Error())
		}
		tokens, err := tx.ExecContext(ctx, "DELETE FROM heimdall.tokens WHERE project = $1", project)
		if err != nil {
			return internal(err.Error())
		}
		if count(secrets)+count(declared)+count(tokens) == 0 {
			return bad(fmt.Sprintf("%s no existe", project))
		}
		return audit(ctx, tx, actor, "project-drop", project, "", nil)
	})
}

// RenameEnvironment moves the derived key and the data with it, so every value
// is sealed again: the old box does not open under the new name.
func (s *Store) RenameEnvironment(ctx context.Context, project, env, to, actor string) error {
	if err := slug(to); err != nil {
		return err
	}
	if env == to {
		return nil
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		moved, err := s.secretsOf(ctx, tx, project, env)
		if err != nil {
			return err
		}
		taken, err := s.secretsOf(ctx, tx, project, to)
		if err != nil {
			return err
		}
		if len(taken) > 0 {
			return bad(fmt.Sprintf("%s/%s ya tiene secretos", project, to))
		}
		subkey := s.key.Derive(secretContext(project, to))
		for name, value := range moved {
			sealed, err := subkey.Seal([]byte(value), []byte(aad(project, to, name)))
			if err != nil {
				return internal(err.Error())
			}
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO heimdall.secrets (project, env, name, value, updated_at) VALUES ($1, $2, $3, $4, $5)",
				project, to, name, sealed, now(),
			); err != nil {
				return internal(err.Error())
			}
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM heimdall.secrets WHERE project = $1 AND env = $2", project, env); err != nil {
			return internal(err.Error())
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM heimdall.environments WHERE project = $1 AND env = $2", project, to); err != nil {
			return internal(err.Error())
		}
		if _, err := tx.ExecContext(ctx, "UPDATE heimdall.environments SET env = $1 WHERE project = $2 AND env = $3", to, project, env); err != nil {
			return internal(err.Error())
		}
		return audit(ctx, tx, actor, "env-rename", project, to, &env)
	})
}

func (s *Store) RenameProject(ctx context.Context, project, to, actor string) error {
	if err := slug(to); err != nil {
		return err
	}
	if project == to {
		return nil
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		names, err := s.namesOf(ctx, tx)
		if err != nil {
			return err
		}
		for _, name := range names {
			if len(name) > len(to)+1 && name[:len(to)+1] == to+"/" {
				return bad(fmt.Sprintf("%s ya existe", to))
			}
		}
		rows, err := tx.QueryContext(ctx, `SELECT env FROM heimdall.environments WHERE project = $1
             UNION
             SELECT DISTINCT env FROM heimdall.secrets WHERE project = $1`, project)
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
			held, err := s.secretsOf(ctx, tx, project, env)
			if err != nil {
				return err
			}
			subkey := s.key.Derive(secretContext(to, env))
			for name, value := range held {
				sealed, err := subkey.Seal([]byte(value), []byte(aad(to, env, name)))
				if err != nil {
					return internal(err.Error())
				}
				if _, err := tx.ExecContext(ctx,
					"INSERT INTO heimdall.secrets (project, env, name, value, updated_at) VALUES ($1, $2, $3, $4, $5)",
					to, env, name, sealed, now(),
				); err != nil {
					return internal(err.Error())
				}
			}
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM heimdall.secrets WHERE project = $1", project); err != nil {
			return internal(err.Error())
		}
		if _, err := tx.ExecContext(ctx, "UPDATE heimdall.environments SET project = $1 WHERE project = $2", to, project); err != nil {
			return internal(err.Error())
		}
		return audit(ctx, tx, actor, "project-rename", to, "", &project)
	})
}

func (s *Store) Secrets(ctx context.Context, project, env string) (map[string]string, error) {
	return s.secretsOf(ctx, s.db, project, env)
}

func (s *Store) secretsOf(ctx context.Context, db querier, project, env string) (map[string]string, error) {
	subkey := s.key.Derive(secretContext(project, env))
	rows, err := db.QueryContext(ctx,
		"SELECT name, value FROM heimdall.secrets WHERE project = $1 AND env = $2", project, env)
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
		plain, err := subkey.Open(value, []byte(aad(project, env, name)))
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

func (s *Store) Set(ctx context.Context, project, env, name, value, actor string) error {
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
	sealed, err := s.key.Derive(secretContext(project, env)).Seal([]byte(value), []byte(aad(project, env, name)))
	if err != nil {
		return internal(err.Error())
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO heimdall.secrets (project, env, name, value, updated_at) VALUES ($1, $2, $3, $4, $5)
             ON CONFLICT (project, env, name) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			project, env, name, sealed, now(),
		); err != nil {
			return internal(err.Error())
		}
		return audit(ctx, tx, actor, "set", project, env, &name)
	})
}

func (s *Store) Unset(ctx context.Context, project, env, name, actor string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		removed, err := tx.ExecContext(ctx,
			"DELETE FROM heimdall.secrets WHERE project = $1 AND env = $2 AND name = $3", project, env, name)
		if err != nil {
			return internal(err.Error())
		}
		if count(removed) == 0 {
			return bad(fmt.Sprintf("%s no está en %s/%s", name, project, env))
		}
		return audit(ctx, tx, actor, "unset", project, env, &name)
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
			`INSERT INTO heimdall.tokens (id, name, project, env, keys, hash, role, created_at, expires_at, last_used)
             VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULL)`,
			token.ID, token.Name, token.Project, token.Env, keys, Hash(plain), token.Role, token.CreatedAt, token.ExpiresAt,
		); err != nil {
			return internal(err.Error())
		}
		return audit(ctx, tx, actor, "token-create", token.Project, token.Env, &token.Name)
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
		return audit(ctx, tx, actor, "token-revoke", token.Project, token.Env, &token.Name)
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

func (s *Store) Audit(ctx context.Context, actor, action, project, env string, name *string) error {
	return audit(ctx, s.db, actor, action, project, env, name)
}

func (s *Store) AuditLog(ctx context.Context, limit int) ([]AuditRow, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT at, actor, action, project, env, name FROM heimdall.audit ORDER BY seq DESC LIMIT $1", limit)
	if err != nil {
		return nil, internal(err.Error())
	}
	defer rows.Close()
	log := []AuditRow{}
	for rows.Next() {
		var row AuditRow
		if err := rows.Scan(&row.At, &row.Actor, &row.Action, &row.Project, &row.Env, &row.Key); err != nil {
			return nil, internal(err.Error())
		}
		log = append(log, row)
	}
	return log, rows.Err()
}

func (s *Store) CreateLogin(ctx context.Context, email string, ttl int64) (string, error) {
	plain, err := RandomHex(24)
	if err != nil {
		return "", internal(err.Error())
	}
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO heimdall.logins (hash, email, created_at, expires_at) VALUES ($1, $2, $3, $4)",
		Hash(plain), email, now(), now()+ttl,
	); err != nil {
		return "", internal(err.Error())
	}
	return plain, nil
}

func (s *Store) ConsumeLogin(ctx context.Context, plain string) (string, error) {
	hash := Hash(plain)
	var email string
	err := s.db.QueryRowContext(ctx,
		"SELECT email FROM heimdall.logins WHERE hash = $1 AND expires_at > $2", hash, now()).Scan(&email)
	if errors.Is(err, sql.ErrNoRows) {
		return "", bad("ese link ya no sirve")
	}
	if err != nil {
		return "", internal(err.Error())
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM heimdall.logins WHERE hash = $1", hash); err != nil {
		return "", internal(err.Error())
	}
	return email, nil
}

func (s *Store) AskedRecently(ctx context.Context, email string, within int64) (bool, error) {
	var last *int64
	if err := s.db.QueryRowContext(ctx,
		"SELECT MAX(created_at) FROM heimdall.logins WHERE email = $1", email).Scan(&last); err != nil {
		return false, internal(err.Error())
	}
	return last != nil && now()-*last < within, nil
}

func (s *Store) CreateSession(ctx context.Context, email string, ttl int64) (string, error) {
	plain, err := RandomHex(24)
	if err != nil {
		return "", internal(err.Error())
	}
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO heimdall.sessions (hash, email, created_at, expires_at) VALUES ($1, $2, $3, $4)",
		Hash(plain), email, now(), now()+ttl,
	); err != nil {
		return "", internal(err.Error())
	}
	return plain, nil
}

func (s *Store) Session(ctx context.Context, plain string) (string, bool, error) {
	var email string
	err := s.db.QueryRowContext(ctx,
		"SELECT email FROM heimdall.sessions WHERE hash = $1 AND expires_at > $2", Hash(plain), now()).Scan(&email)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, internal(err.Error())
	}
	return email, true, nil
}

func (s *Store) DropSession(ctx context.Context, plain string) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM heimdall.sessions WHERE hash = $1", Hash(plain)); err != nil {
		return internal(err.Error())
	}
	return nil
}

func (s *Store) Sweep(ctx context.Context) error {
	at := now()
	if _, err := s.db.ExecContext(ctx, "DELETE FROM heimdall.logins WHERE expires_at <= $1", at); err != nil {
		return internal(err.Error())
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM heimdall.sessions WHERE expires_at <= $1", at); err != nil {
		return internal(err.Error())
	}
	return nil
}

func (s *Store) namesOf(ctx context.Context, db querier) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT project || '/' || env FROM heimdall.environments
         UNION
         SELECT DISTINCT project || '/' || env FROM heimdall.secrets
         ORDER BY 1`)
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

func audit(ctx context.Context, db querier, actor, action, project, env string, name *string) error {
	if _, err := db.ExecContext(ctx,
		"INSERT INTO heimdall.audit (at, actor, action, project, env, name) VALUES ($1, $2, $3, $4, $5, $6)",
		now(), actor, action, project, env, name,
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
	targets := []any{&token.ID, &token.Name, &token.Project, &token.Env, &keys, &token.Role,
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

func secretContext(project, env string) string {
	return fmt.Sprintf("secrets/%s/%s", project, env)
}

// The sealed box carries its own name inside, so moving it to another row does
// not turn it into another secret.
func aad(project, env, name string) string {
	return fmt.Sprintf("secrets/%s/%s/%s", project, env, name)
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
