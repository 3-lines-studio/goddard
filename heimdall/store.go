package heimdall

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

const MaxValue = 64 * 1024

const schema = `
CREATE TABLE IF NOT EXISTS secrets (
    project TEXT NOT NULL,
    env TEXT NOT NULL,
    name TEXT NOT NULL,
    value BLOB NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (project, env, name)
);
CREATE TABLE IF NOT EXISTS tokens (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    project TEXT NOT NULL,
    env TEXT NOT NULL,
    keys TEXT,
    hash TEXT NOT NULL,
    admin INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    expires_at INTEGER,
    last_used INTEGER
);
CREATE INDEX IF NOT EXISTS tokens_hash ON tokens (hash);
CREATE TABLE IF NOT EXISTS audit (
    at INTEGER NOT NULL,
    actor TEXT NOT NULL,
    action TEXT NOT NULL,
    project TEXT NOT NULL,
    env TEXT NOT NULL,
    name TEXT
);
CREATE TABLE IF NOT EXISTS environments (
    project TEXT NOT NULL,
    env TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (project, env)
);
CREATE TABLE IF NOT EXISTS logins (
    hash TEXT PRIMARY KEY,
    email TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
    hash TEXT PRIMARY KEY,
    email TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);
`

const tokenColumns = "id, name, project, env, keys, admin, created_at, expires_at, last_used"

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
	Admin     bool     `json:"admin"`
	CreatedAt int64    `json:"created_at"`
	ExpiresAt *int64   `json:"expires_at"`
	LastUsed  *int64   `json:"last_used"`
}

type NewToken struct {
	Name    string
	Project string
	Env     string
	Keys    []string
	Admin   bool
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

func OpenStore(path string, key Key) (*Store, error) {
	if parent := filepath.Dir(path); parent != "." {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return nil, fmt.Errorf("no pude crear %q: %v", parent, err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("no pude abrir %q: %v", path, err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("no pude poner %q en WAL: %v", path, err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("no pude armar el esquema: %v", err)
	}
	return &Store{db: db, key: key}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// Names is what was declared plus what already has secrets: the environments
// that existed before there was a table still show up, with nothing migrated.
func (s *Store) Names() ([]string, error) {
	rows, err := s.db.Query(`SELECT project || '/' || env FROM environments
         UNION
         SELECT DISTINCT project || '/' || env FROM secrets
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

func (s *Store) CreateEnvironment(project, env, actor string) error {
	if err := slug(project); err != nil {
		return err
	}
	if err := slug(env); err != nil {
		return err
	}
	_, err := s.db.Exec(
		"INSERT OR IGNORE INTO environments (project, env, created_at) VALUES (?, ?, ?)",
		project, env, now())
	if err != nil {
		return internal(err.Error())
	}
	return s.Audit(actor, "env-create", project, env, nil)
}

// DropEnvironment takes the tokens that pointed at the environment with it:
// they make no sense over something that stopped existing.
func (s *Store) DropEnvironment(project, env, actor string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return internal(err.Error())
	}
	defer tx.Rollback()
	secrets, err := tx.Exec("DELETE FROM secrets WHERE project = ? AND env = ?", project, env)
	if err != nil {
		return internal(err.Error())
	}
	declared, err := tx.Exec("DELETE FROM environments WHERE project = ? AND env = ?", project, env)
	if err != nil {
		return internal(err.Error())
	}
	tokens, err := tx.Exec("DELETE FROM tokens WHERE project = ? AND env = ?", project, env)
	if err != nil {
		return internal(err.Error())
	}
	removed := count(secrets) + count(declared) + count(tokens)
	if removed == 0 {
		return bad(fmt.Sprintf("%s/%s no existe", project, env))
	}
	if err := audit(tx, actor, "env-drop", project, env, nil); err != nil {
		return err
	}
	return commit(tx)
}

func (s *Store) DropProject(project, actor string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return internal(err.Error())
	}
	defer tx.Rollback()
	secrets, err := tx.Exec("DELETE FROM secrets WHERE project = ?", project)
	if err != nil {
		return internal(err.Error())
	}
	declared, err := tx.Exec("DELETE FROM environments WHERE project = ?", project)
	if err != nil {
		return internal(err.Error())
	}
	tokens, err := tx.Exec("DELETE FROM tokens WHERE project = ?", project)
	if err != nil {
		return internal(err.Error())
	}
	removed := count(secrets) + count(declared) + count(tokens)
	if removed == 0 {
		return bad(fmt.Sprintf("%s no existe", project))
	}
	if err := audit(tx, actor, "project-drop", project, "", nil); err != nil {
		return err
	}
	return commit(tx)
}

// RenameEnvironment moves the derived key and the data with it, so every value
// is sealed again: the old box does not open under the new name.
func (s *Store) RenameEnvironment(project, env, to, actor string) error {
	if err := slug(to); err != nil {
		return err
	}
	if env == to {
		return nil
	}
	moved, err := s.Secrets(project, env)
	if err != nil {
		return err
	}
	taken, err := s.Secrets(project, to)
	if err != nil {
		return err
	}
	if len(taken) > 0 {
		return bad(fmt.Sprintf("%s/%s ya tiene secretos", project, to))
	}
	tx, err := s.db.Begin()
	if err != nil {
		return internal(err.Error())
	}
	defer tx.Rollback()
	subkey := s.key.Derive(context(project, to))
	for name, value := range moved {
		sealed, err := subkey.Seal([]byte(value), []byte(aad(project, to, name)))
		if err != nil {
			return internal(err.Error())
		}
		if _, err := tx.Exec(
			"INSERT INTO secrets (project, env, name, value, updated_at) VALUES (?, ?, ?, ?, ?)",
			project, to, name, sealed, now(),
		); err != nil {
			return internal(err.Error())
		}
	}
	if _, err := tx.Exec("DELETE FROM secrets WHERE project = ? AND env = ?", project, env); err != nil {
		return internal(err.Error())
	}
	if _, err := tx.Exec("DELETE FROM environments WHERE project = ? AND env = ?", project, to); err != nil {
		return internal(err.Error())
	}
	if _, err := tx.Exec("UPDATE environments SET env = ? WHERE project = ? AND env = ?", to, project, env); err != nil {
		return internal(err.Error())
	}
	if err := audit(tx, actor, "env-rename", project, to, &env); err != nil {
		return err
	}
	return commit(tx)
}

func (s *Store) RenameProject(project, to, actor string) error {
	if err := slug(to); err != nil {
		return err
	}
	if project == to {
		return nil
	}
	names, err := s.Names()
	if err != nil {
		return err
	}
	for _, name := range names {
		if len(name) > len(to)+1 && name[:len(to)+1] == to+"/" {
			return bad(fmt.Sprintf("%s ya existe", to))
		}
	}
	rows, err := s.db.Query(`SELECT env FROM environments WHERE project = ?
         UNION
         SELECT DISTINCT env FROM secrets WHERE project = ?`, project, project)
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
	held := map[string]map[string]string{}
	for _, env := range envs {
		secrets, err := s.Secrets(project, env)
		if err != nil {
			return err
		}
		held[env] = secrets
	}
	tx, err := s.db.Begin()
	if err != nil {
		return internal(err.Error())
	}
	defer tx.Rollback()
	for _, env := range envs {
		subkey := s.key.Derive(context(to, env))
		for name, value := range held[env] {
			sealed, err := subkey.Seal([]byte(value), []byte(aad(to, env, name)))
			if err != nil {
				return internal(err.Error())
			}
			if _, err := tx.Exec(
				"INSERT INTO secrets (project, env, name, value, updated_at) VALUES (?, ?, ?, ?, ?)",
				to, env, name, sealed, now(),
			); err != nil {
				return internal(err.Error())
			}
		}
	}
	if _, err := tx.Exec("DELETE FROM secrets WHERE project = ?", project); err != nil {
		return internal(err.Error())
	}
	if _, err := tx.Exec("UPDATE environments SET project = ? WHERE project = ?", to, project); err != nil {
		return internal(err.Error())
	}
	if err := audit(tx, actor, "project-rename", to, "", &project); err != nil {
		return err
	}
	return commit(tx)
}

func (s *Store) Secrets(project, env string) (map[string]string, error) {
	subkey := s.key.Derive(context(project, env))
	rows, err := s.db.Query("SELECT name, value FROM secrets WHERE project = ? AND env = ?", project, env)
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

func (s *Store) Set(project, env, name, value, actor string) error {
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
	sealed, err := s.key.Derive(context(project, env)).Seal([]byte(value), []byte(aad(project, env, name)))
	if err != nil {
		return internal(err.Error())
	}
	tx, err := s.db.Begin()
	if err != nil {
		return internal(err.Error())
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`INSERT INTO secrets (project, env, name, value, updated_at) VALUES (?, ?, ?, ?, ?)
         ON CONFLICT (project, env, name) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		project, env, name, sealed, now(),
	); err != nil {
		return internal(err.Error())
	}
	if err := audit(tx, actor, "set", project, env, &name); err != nil {
		return err
	}
	return commit(tx)
}

func (s *Store) Unset(project, env, name, actor string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return internal(err.Error())
	}
	defer tx.Rollback()
	removed, err := tx.Exec("DELETE FROM secrets WHERE project = ? AND env = ? AND name = ?", project, env, name)
	if err != nil {
		return internal(err.Error())
	}
	if count(removed) == 0 {
		return bad(fmt.Sprintf("%s no está en %s/%s", name, project, env))
	}
	if err := audit(tx, actor, "unset", project, env, &name); err != nil {
		return err
	}
	return commit(tx)
}

func (s *Store) Tokens() ([]Token, error) {
	rows, err := s.db.Query(fmt.Sprintf("SELECT %s FROM tokens ORDER BY created_at", tokenColumns))
	if err != nil {
		return nil, internal(err.Error())
	}
	defer rows.Close()
	return scanTokens(rows)
}

func (s *Store) CreateToken(new NewToken, actor string) (Token, string, error) {
	if new.Admin {
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
	plain = "hd_" + plain
	token := Token{
		ID:        id,
		Name:      new.Name,
		Project:   new.Project,
		Env:       new.Env,
		Keys:      new.Keys,
		Admin:     new.Admin,
		CreatedAt: now(),
	}
	if new.TTL != nil {
		expires := now() + *new.TTL
		token.ExpiresAt = &expires
	}
	var keys *string
	if token.Keys != nil {
		encoded, err := json.Marshal(token.Keys)
		if err != nil {
			return Token{}, "", internal(err.Error())
		}
		text := string(encoded)
		keys = &text
	}
	tx, err := s.db.Begin()
	if err != nil {
		return Token{}, "", internal(err.Error())
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`INSERT INTO tokens (id, name, project, env, keys, hash, admin, created_at, expires_at, last_used)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		token.ID, token.Name, token.Project, token.Env, keys, Hash(plain), token.Admin, token.CreatedAt, token.ExpiresAt,
	); err != nil {
		return Token{}, "", internal(err.Error())
	}
	if err := audit(tx, actor, "token-create", token.Project, token.Env, &token.Name); err != nil {
		return Token{}, "", err
	}
	if err := commit(tx); err != nil {
		return Token{}, "", err
	}
	return token, plain, nil
}

func (s *Store) Revoke(id, actor string) (Token, error) {
	tokens, err := s.Tokens()
	if err != nil {
		return Token{}, err
	}
	var found *Token
	for i := range tokens {
		if tokens[i].ID == id {
			found = &tokens[i]
			break
		}
	}
	if found == nil {
		return Token{}, bad(fmt.Sprintf("no hay token %s", id))
	}
	token := *found
	tx, err := s.db.Begin()
	if err != nil {
		return Token{}, internal(err.Error())
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM tokens WHERE id = ?", id); err != nil {
		return Token{}, internal(err.Error())
	}
	if err := audit(tx, actor, "token-revoke", token.Project, token.Env, &token.Name); err != nil {
		return Token{}, err
	}
	if err := commit(tx); err != nil {
		return Token{}, err
	}
	return token, nil
}

func (s *Store) Find(plain string) (*Token, error) {
	presented := Hash(plain)
	rows, err := s.db.Query(fmt.Sprintf("SELECT %s, hash FROM tokens", tokenColumns))
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

func (s *Store) Touch(id string) error {
	if _, err := s.db.Exec("UPDATE tokens SET last_used = ? WHERE id = ?", now(), id); err != nil {
		return internal(err.Error())
	}
	return nil
}

func (s *Store) Audit(actor, action, project, env string, name *string) error {
	return audit(s.db, actor, action, project, env, name)
}

func (s *Store) AuditLog(limit int) ([]AuditRow, error) {
	rows, err := s.db.Query(
		"SELECT at, actor, action, project, env, name FROM audit ORDER BY at DESC, rowid DESC LIMIT ?", limit)
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

func (s *Store) CreateLogin(email string, ttl int64) (string, error) {
	plain, err := RandomHex(24)
	if err != nil {
		return "", internal(err.Error())
	}
	if _, err := s.db.Exec(
		"INSERT INTO logins (hash, email, created_at, expires_at) VALUES (?, ?, ?, ?)",
		Hash(plain), email, now(), now()+ttl,
	); err != nil {
		return "", internal(err.Error())
	}
	return plain, nil
}

func (s *Store) ConsumeLogin(plain string) (string, error) {
	hash := Hash(plain)
	var email string
	err := s.db.QueryRow(
		"SELECT email FROM logins WHERE hash = ? AND expires_at > ?", hash, now()).Scan(&email)
	if errors.Is(err, sql.ErrNoRows) {
		return "", bad("ese link ya no sirve")
	}
	if err != nil {
		return "", internal(err.Error())
	}
	if _, err := s.db.Exec("DELETE FROM logins WHERE hash = ?", hash); err != nil {
		return "", internal(err.Error())
	}
	return email, nil
}

func (s *Store) AskedRecently(email string, within int64) (bool, error) {
	var last *int64
	if err := s.db.QueryRow(
		"SELECT MAX(created_at) FROM logins WHERE email = ?", email).Scan(&last); err != nil {
		return false, internal(err.Error())
	}
	return last != nil && now()-*last < within, nil
}

func (s *Store) CreateSession(email string, ttl int64) (string, error) {
	plain, err := RandomHex(24)
	if err != nil {
		return "", internal(err.Error())
	}
	if _, err := s.db.Exec(
		"INSERT INTO sessions (hash, email, created_at, expires_at) VALUES (?, ?, ?, ?)",
		Hash(plain), email, now(), now()+ttl,
	); err != nil {
		return "", internal(err.Error())
	}
	return plain, nil
}

func (s *Store) Session(plain string) (string, bool, error) {
	var email string
	err := s.db.QueryRow(
		"SELECT email FROM sessions WHERE hash = ? AND expires_at > ?", Hash(plain), now()).Scan(&email)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, internal(err.Error())
	}
	return email, true, nil
}

func (s *Store) DropSession(plain string) error {
	if _, err := s.db.Exec("DELETE FROM sessions WHERE hash = ?", Hash(plain)); err != nil {
		return internal(err.Error())
	}
	return nil
}

func (s *Store) Sweep() error {
	at := now()
	if _, err := s.db.Exec("DELETE FROM logins WHERE expires_at <= ?", at); err != nil {
		return internal(err.Error())
	}
	if _, err := s.db.Exec("DELETE FROM sessions WHERE expires_at <= ?", at); err != nil {
		return internal(err.Error())
	}
	return nil
}

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func audit(db execer, actor, action, project, env string, name *string) error {
	if _, err := db.Exec(
		"INSERT INTO audit (at, actor, action, project, env, name) VALUES (?, ?, ?, ?, ?, ?)",
		now(), actor, action, project, env, name,
	); err != nil {
		return internal(err.Error())
	}
	return nil
}

func scanTokens(rows *sql.Rows) ([]Token, error) {
	tokens := []Token{}
	for rows.Next() {
		token, _, err := scanToken(rows, false)
		if err != nil {
			return nil, internal(err.Error())
		}
		tokens = append(tokens, token)
	}
	return tokens, rows.Err()
}

func scanToken(row interface{ Scan(...any) error }, withHash bool) (Token, string, error) {
	var token Token
	var keys *string
	var admin int64
	var hash string
	targets := []any{&token.ID, &token.Name, &token.Project, &token.Env, &keys, &admin,
		&token.CreatedAt, &token.ExpiresAt, &token.LastUsed}
	if withHash {
		targets = append(targets, &hash)
	}
	if err := row.Scan(targets...); err != nil {
		return Token{}, "", err
	}
	if keys != nil {
		json.Unmarshal([]byte(*keys), &token.Keys)
	}
	token.Admin = admin != 0
	return token, hash, nil
}

func commit(tx *sql.Tx) error {
	if err := tx.Commit(); err != nil {
		return internal(err.Error())
	}
	return nil
}

func count(result sql.Result) int64 {
	rows, err := result.RowsAffected()
	if err != nil {
		return 0
	}
	return rows
}

func context(project, env string) string {
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
