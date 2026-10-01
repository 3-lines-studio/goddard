package heimdall

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

const siglo = 100 * 365 * 24 * 60 * 60

// legacySchema is the schema the Rust crate left behind, verbatim.
const legacySchema = `
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

// fixture goes the long way around on purpose: it builds the store the Rust
// wrote, in SQLite, and imports it. What the tests see afterwards is what a
// real migration would leave in Postgres.
func fixture(t *testing.T) *Store {
	t.Helper()
	store := testStore(t)
	imported, err := ImportSQLite(t.Context(), buildLegacy(t), store)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	want := Imported{Secrets: 6, Environments: 1, Tokens: 6, Audit: 17}
	if imported != want {
		t.Fatalf("importó %+v", imported)
	}
	return store
}

func buildLegacy(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "heimdall.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("no pude abrir el SQLite: %v", err)
	}
	if _, err := db.Exec(legacySchema); err != nil {
		t.Fatalf("no pude armar el esquema viejo: %v", err)
	}
	raw, err := os.ReadFile("testdata/rust-store.sql")
	if err != nil {
		t.Fatalf("testdata: %v", err)
	}
	inserts := 0
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		if !strings.HasPrefix(line, "INSERT INTO ") {
			t.Fatalf("testdata: línea que no es un INSERT: %q", line)
		}
		if _, err := db.Exec(line); err != nil {
			t.Fatalf("testdata: %v", err)
		}
		inserts++
	}
	if inserts == 0 {
		t.Fatal("testdata: no trae ningún INSERT")
	}
	if err := db.Close(); err != nil {
		t.Fatalf("no pude cerrar el SQLite: %v", err)
	}
	return path
}

func TestParidadDelStoreConRust(t *testing.T) {
	store := fixture(t)

	t.Run("el listado", func(t *testing.T) {
		want := []string{"axe/dev", "bifrost/dev", "bifrost/prod", "nueva/dev", "picsel/dev", "viejo/dev"}
		if got := names(t, store); !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
	})

	t.Run("los valores", func(t *testing.T) {
		casos := []struct {
			project, env string
			want         map[string]string
		}{
			{"axe", "dev", map[string]string{"A": "1"}},
			{"bifrost", "dev", map[string]string{"STRIPE_KEY": "sk_test_123", "DB_PASSWORD": "hunter2"}},
			{"bifrost", "prod", map[string]string{"STRIPE_KEY": "sk_live_1"}},
			{"picsel", "dev", map[string]string{"BUCKET": "picsel-staging"}},
			{"nueva", "dev", map[string]string{}},
			{"borrado", "dev", map[string]string{}},
		}
		for _, caso := range casos {
			got, err := store.Secrets(t.Context(), caso.project, caso.env)
			if err != nil {
				t.Fatalf("%s/%s: %v", caso.project, caso.env, err)
			}
			if !reflect.DeepEqual(got, caso.want) {
				t.Errorf("%s/%s:\n got:  %v\n want: %v", caso.project, caso.env, got, caso.want)
			}
		}
	})

	t.Run("un entorno renombrado se vuelve a sellar", func(t *testing.T) {
		got, err := store.Secrets(t.Context(), "viejo", "dev")
		if err != nil {
			t.Fatalf("secrets: %v", err)
		}
		if got["CLAVE"] != "se-mueve" {
			t.Fatalf("el valor renombrado quedó en %q", got["CLAVE"])
		}
		old, err := store.Secrets(t.Context(), "viejo", "qa")
		if err != nil {
			t.Fatalf("secrets: %v", err)
		}
		if len(old) != 0 {
			t.Fatalf("el nombre viejo tiene %v", old)
		}
	})

	t.Run("los tokens", func(t *testing.T) {
		dump := readTestdata(t, "testdata/paridad-store.txt")
		casos := []struct {
			name, project, env, role string
			keys                     []string
		}{
			{"agente", "bifrost", "dev", RoleAgent, []string{"STRIPE_KEY"}},
			{"runner", "*", "dev", RoleAgent, nil},
			{"viejo", "bifrost", "prod", RoleAgent, nil},
			{"con_ttl", "axe", "dev", RoleAgent, nil},
			{"jimmy", "", "", RoleAdmin, nil},
		}
		for _, caso := range casos {
			token, err := store.Find(t.Context(), dump["token_"+caso.name])
			if err != nil {
				t.Fatalf("%s: %v", caso.name, err)
			}
			if token == nil {
				t.Fatalf("%s: no lo encontró", caso.name)
			}
			if token.Name != caso.name || token.Project != caso.project || token.Env != caso.env || token.Role != caso.role {
				t.Errorf("%s: quedó %s %s/%s rol=%s", caso.name, token.Name, token.Project, token.Env, token.Role)
			}
			if !reflect.DeepEqual(token.Keys, caso.keys) {
				t.Errorf("%s: claves %v", caso.name, token.Keys)
			}
		}
		if _, err := store.Find(t.Context(), dump["token_vencido"]); err == nil {
			t.Fatal("el token vencido entró")
		} else if kind(err) != ErrBad {
			t.Fatalf("el token vencido dio %v", err)
		}
		tokens, err := store.Tokens(t.Context())
		if err != nil {
			t.Fatalf("tokens: %v", err)
		}
		if len(tokens) != 6 {
			t.Fatalf("la base tiene %d tokens", len(tokens))
		}
		for _, token := range tokens {
			if token.Name != "con_ttl" {
				continue
			}
			if token.ExpiresAt == nil || *token.ExpiresAt-token.CreatedAt != siglo {
				t.Fatalf("el ttl quedó en %v", token.ExpiresAt)
			}
		}
	})

	t.Run("el audit", func(t *testing.T) {
		log, err := store.AuditLog(t.Context(), 500)
		if err != nil {
			t.Fatalf("audit: %v", err)
		}
		got := []string{}
		for _, row := range log {
			key := "-"
			if row.Key != nil {
				key = *row.Key
			}
			got = append(got, fmt.Sprintf("%s %s %s/%s %s", row.Action, row.Actor, row.Project, row.Env, key))
		}
		want := []string{
			"get-secrets berti bifrost/dev -",
			"token-create berti / jimmy",
			"token-create berti axe/dev vencido",
			"token-create berti axe/dev con_ttl",
			"token-create berti bifrost/prod viejo",
			"token-create berti */dev runner",
			"token-create berti bifrost/dev agente",
			"env-drop berti borrado/dev -",
			"set berti borrado/dev NO_DEBE_QUEDAR",
			"env-rename berti viejo/dev qa",
			"set berti viejo/qa CLAVE",
			"env-create berti nueva/dev -",
			"set berti picsel/dev BUCKET",
			"set berti axe/dev A",
			"set berti bifrost/prod STRIPE_KEY",
			"set berti bifrost/dev DB_PASSWORD",
			"set berti bifrost/dev STRIPE_KEY",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("el audit quedó:\n%s\nwant:\n%s", join(got), join(want))
		}
	})

}

func TestElImportadorNoSePisaDosVeces(t *testing.T) {
	store := testStore(t)
	if _, err := ImportSQLite(t.Context(), buildLegacy(t), store); err != nil {
		t.Fatalf("import: %v", err)
	}
	if _, err := ImportSQLite(t.Context(), buildLegacy(t), store); err == nil {
		t.Fatal("importó dos veces lo mismo sin quejarse")
	}
}

func kind(err error) ErrorKind {
	var typed *Error
	if errors.As(err, &typed) {
		return typed.Kind
	}
	return ErrInternal
}

func join(lines []string) string {
	out := ""
	for _, line := range lines {
		out += "\t" + line + "\n"
	}
	return out
}
