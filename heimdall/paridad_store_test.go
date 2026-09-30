package heimdall

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fixture arma la base que escribió el Rust: cada INSERT trae los bytes tal
// cual salieron de allá, sobres y hashes incluidos.
func fixture(t *testing.T) *Store {
	t.Helper()
	store := storeAt(t, filepath.Join(t.TempDir(), "heimdall.db"))
	loadFixture(t, store, "testdata/rust-store.sql")
	return store
}

func loadFixture(t *testing.T, store *Store, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("testdata %s: %v", path, err)
	}
	inserts := 0
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		if !strings.HasPrefix(line, "INSERT INTO ") {
			t.Fatalf("testdata %s: línea que no es un INSERT: %q", path, line)
		}
		if _, err := store.db.Exec(line); err != nil {
			t.Fatalf("testdata %s: %v", path, err)
		}
		inserts++
	}
	if inserts == 0 {
		t.Fatalf("testdata %s: no trae ningún INSERT", path)
	}
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
			got, err := store.Secrets(caso.project, caso.env)
			if err != nil {
				t.Fatalf("%s/%s: %v", caso.project, caso.env, err)
			}
			if !reflect.DeepEqual(got, caso.want) {
				t.Errorf("%s/%s:\n got:  %v\n want: %v", caso.project, caso.env, got, caso.want)
			}
		}
	})

	t.Run("un entorno renombrado se vuelve a sellar", func(t *testing.T) {
		got, err := store.Secrets("viejo", "dev")
		if err != nil {
			t.Fatalf("secrets: %v", err)
		}
		if got["CLAVE"] != "se-mueve" {
			t.Fatalf("el valor renombrado quedó en %q", got["CLAVE"])
		}
		old, err := store.Secrets("viejo", "qa")
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
			name, project, env string
			keys               []string
			admin              bool
		}{
			{"agente", "bifrost", "dev", []string{"STRIPE_KEY"}, false},
			{"runner", "*", "dev", nil, false},
			{"viejo", "bifrost", "prod", nil, false},
			{"con_ttl", "axe", "dev", nil, false},
			{"jimmy", "", "", nil, true},
		}
		for _, caso := range casos {
			token, err := store.Find(dump["token_"+caso.name])
			if err != nil {
				t.Fatalf("%s: %v", caso.name, err)
			}
			if token == nil {
				t.Fatalf("%s: no lo encontró", caso.name)
			}
			if token.Name != caso.name || token.Project != caso.project || token.Env != caso.env || token.Admin != caso.admin {
				t.Errorf("%s: quedó %s %s/%s admin=%v", caso.name, token.Name, token.Project, token.Env, token.Admin)
			}
			if !reflect.DeepEqual(token.Keys, caso.keys) {
				t.Errorf("%s: claves %v", caso.name, token.Keys)
			}
		}
		if _, err := store.Find(dump["token_vencido"]); err == nil {
			t.Fatal("el token vencido entró")
		} else if kind(err) != ErrBad {
			t.Fatalf("el token vencido dio %v", err)
		}
		tokens, err := store.Tokens()
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
			if token.ExpiresAt == nil || *token.ExpiresAt-token.CreatedAt != 3600 {
				t.Fatalf("el ttl quedó en %v", token.ExpiresAt)
			}
		}
	})

	t.Run("el audit", func(t *testing.T) {
		log, err := store.AuditLog(500)
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

	t.Run("los links y las sesiones", func(t *testing.T) {
		dump := readTestdata(t, "testdata/paridad-store.txt")
		email, err := store.ConsumeLogin(dump["login_vivo"])
		if err != nil {
			t.Fatalf("login vivo: %v", err)
		}
		if email != "berti@ejemplo.com" {
			t.Fatalf("el link dio %q", email)
		}
		if _, err := store.ConsumeLogin(dump["login_vencido"]); err == nil {
			t.Fatal("el link vencido sirvió")
		}
		got, ok, err := store.Session(dump["session_viva"])
		if err != nil {
			t.Fatalf("sesión viva: %v", err)
		}
		if !ok || got != "berti@ejemplo.com" {
			t.Fatalf("la sesión dio %q %v", got, ok)
		}
		if _, ok, err := store.Session(dump["session_vencida"]); err != nil || ok {
			t.Fatalf("la sesión vencida entró: %v %v", ok, err)
		}
		asked, err := store.AskedRecently("berti@ejemplo.com", 100*365*24*60*60)
		if err != nil {
			t.Fatalf("asked: %v", err)
		}
		if !asked {
			t.Fatal("no vio los links que ya estaban")
		}
		asked, err = store.AskedRecently("otro@ejemplo.com", 100*365*24*60*60)
		if err != nil {
			t.Fatalf("asked: %v", err)
		}
		if asked {
			t.Fatal("le pegó el cooldown de otra dirección")
		}
	})
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
