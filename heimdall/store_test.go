package heimdall

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/3-lines-studio/goddard/migrations"
)

const testLock = 0x676f6464544553

// testDB deja una base limpia con el esquema aplicado. El candado es para que
// los paquetes que corren en paralelo no se pisen el schema.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("sin TEST_DATABASE_URL no hay Postgres contra el que correr")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatalf("no pude abrir %s: %v", url, err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatalf("no pude hablar con %s: %v", url, err)
	}
	lock, err := db.Conn(t.Context())
	if err != nil {
		t.Fatalf("no pude reservar una conexión: %v", err)
	}
	t.Cleanup(func() {
		lock.ExecContext(context.WithoutCancel(t.Context()), "SELECT pg_advisory_unlock($1)", testLock)
		lock.Close()
	})
	if _, err := lock.ExecContext(t.Context(), "SELECT pg_advisory_lock($1)", testLock); err != nil {
		t.Fatalf("no pude tomar el candado: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), "DROP SCHEMA IF EXISTS goddard, auth, chat, heimdall, axe, skill, memo, schedule, org CASCADE; DROP TABLE IF EXISTS public.schema_migrations"); err != nil {
		t.Fatalf("no pude limpiar: %v", err)
	}
	if _, err := migrations.Apply(t.Context(), db); err != nil {
		t.Fatalf("no pude migrar: %v", err)
	}
	return db
}

func testStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(testDB(t), testKey(t))
}

var testOwner = Org("acme")

func storeAt(t *testing.T, path string) *Store {
	t.Helper()
	return testStore(t)
}

func newToken(name, project, env string) NewToken {
	return NewToken{Owner: testOwner, Name: name, Project: project, Env: env}
}

func agent(t *testing.T, store *Store) string {
	t.Helper()
	_, plain, err := store.CreateToken(t.Context(), newToken("agente", "bifrost", "dev"), "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	return plain
}

func names(t *testing.T, store *Store) []string {
	t.Helper()
	got, err := store.Names(t.Context(), testOwner)
	if err != nil {
		t.Fatalf("names: %v", err)
	}
	return got
}

func storedValue(t *testing.T, store *Store, project, env, name string) []byte {
	t.Helper()
	var value []byte
	err := store.db.QueryRowContext(t.Context(),
		"SELECT value FROM heimdall.secrets WHERE project = $1 AND env = $2 AND name = $3",
		project, env, name).Scan(&value)
	if err != nil {
		t.Fatalf("no pude leer el valor guardado: %v", err)
	}
	return value
}

func storedText(t *testing.T, store *Store, query string) string {
	t.Helper()
	rows, err := store.db.QueryContext(t.Context(), query)
	if err != nil {
		t.Fatalf("no pude leer: %v", err)
	}
	defer rows.Close()
	out := ""
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			t.Fatalf("no pude leer: %v", err)
		}
		out += text + "\n"
	}
	return out
}

func TestASecretSurvivesAReopen(t *testing.T) {
	store := testStore(t)
	if err := store.Set(t.Context(), testOwner, "bifrost", "dev", "STRIPE_KEY", "sk_test_123", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	reopened := NewStore(store.db, testKey(t))
	secrets, err := reopened.Secrets(t.Context(), testOwner, "bifrost", "dev")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if secrets["STRIPE_KEY"] != "sk_test_123" {
		t.Fatalf("leyó %q", secrets["STRIPE_KEY"])
	}
}

func TestTheValueNeverLandsInClear(t *testing.T) {
	store := testStore(t)
	if err := store.Set(t.Context(), testOwner, "bifrost", "dev", "DB_PASSWORD", "hunter2", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	raw := storedValue(t, store, "bifrost", "dev", "DB_PASSWORD")
	if bytes.Contains(raw, []byte("hunter2")) {
		t.Fatal("el valor quedó en claro en la base")
	}
}

func TestEnvironmentsAndProjectsDoNotMix(t *testing.T) {
	store := testStore(t)
	for _, caso := range []struct {
		project, env, value string
	}{
		{"bifrost", "dev", "1"},
		{"bifrost", "prod", "2"},
		{"axe", "dev", "3"},
	} {
		if err := store.Set(t.Context(), testOwner, caso.project, caso.env, "A", caso.value, "berti"); err != nil {
			t.Fatalf("set: %v", err)
		}
	}
	for _, caso := range []struct {
		project, env, value string
	}{
		{"bifrost", "dev", "1"},
		{"bifrost", "prod", "2"},
		{"axe", "dev", "3"},
	} {
		secrets, err := store.Secrets(t.Context(), testOwner, caso.project, caso.env)
		if err != nil {
			t.Fatalf("secrets: %v", err)
		}
		if secrets["A"] != caso.value {
			t.Fatalf("%s/%s dio %q", caso.project, caso.env, secrets["A"])
		}
	}
}

func TestBadNamesAreRejected(t *testing.T) {
	store := testStore(t)
	casos := []struct {
		project, env, name string
	}{
		{"../etc", "dev", "A"},
		{"bifrost", "..", "A"},
		{"bifrost", "dev", "A B"},
		{"bifrost", "dev", "1ABC"},
	}
	for _, caso := range casos {
		if err := store.Set(t.Context(), testOwner, caso.project, caso.env, caso.name, "1", "berti"); err == nil {
			t.Fatalf("aceptó %s/%s/%s", caso.project, caso.env, caso.name)
		}
	}
}

func TestMissingSecretsAreAnEmptyMap(t *testing.T) {
	secrets, err := testStore(t).Secrets(t.Context(), testOwner, "bifrost", "prod")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if len(secrets) != 0 {
		t.Fatalf("devolvió %v", secrets)
	}
}

func TestARemovedKeyIsNoLongerThere(t *testing.T) {
	store := testStore(t)
	if err := store.Set(t.Context(), testOwner, "bifrost", "dev", "A", "1", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.Unset(t.Context(), testOwner, "bifrost", "dev", "A", "berti"); err != nil {
		t.Fatalf("unset: %v", err)
	}
	secrets, err := store.Secrets(t.Context(), testOwner, "bifrost", "dev")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if len(secrets) != 0 {
		t.Fatalf("quedó %v", secrets)
	}
	if err := store.Unset(t.Context(), testOwner, "bifrost", "dev", "A", "berti"); err == nil {
		t.Fatal("sacó dos veces la misma clave")
	}
}

func TestAFailedUnsetLeavesNoAuditRow(t *testing.T) {
	store := testStore(t)
	if err := store.Unset(t.Context(), testOwner, "bifrost", "dev", "A", "berti"); err == nil {
		t.Fatal("sacó algo que no estaba")
	}
	log, err := store.AuditLog(t.Context(), 10)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if len(log) != 0 {
		t.Fatalf("dejó %d filas", len(log))
	}
}

func TestOnlyTheMatchingTokenIsFound(t *testing.T) {
	store := testStore(t)
	token, plain, err := store.CreateToken(t.Context(), newToken("agente", "bifrost", "dev"), "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	found, err := store.Find(t.Context(), "hd_nada")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if found != nil {
		t.Fatal("encontró un token que no existe")
	}
	found, err = store.Find(t.Context(), plain)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if found == nil || found.ID != token.ID {
		t.Fatal("no encontró el token")
	}
}

func TestARevokedTokenIsGone(t *testing.T) {
	store := testStore(t)
	token, plain, err := store.CreateToken(t.Context(), newToken("agente", "bifrost", "dev"), "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if _, err := store.Revoke(t.Context(), token.ID, "berti"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	found, err := store.Find(t.Context(), plain)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if found != nil {
		t.Fatal("el token revocado sigue entrando")
	}
}

func TestAnExpiredTokenIsRejected(t *testing.T) {
	store := testStore(t)
	expired := int64(-1)
	new := newToken("agente", "bifrost", "dev")
	new.TTL = &expired
	_, plain, err := store.CreateToken(t.Context(), new, "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if _, err := store.Find(t.Context(), plain); err == nil {
		t.Fatal("el token vencido entró")
	}
}

func TestATokenWithoutTTLDoesNotExpire(t *testing.T) {
	store := testStore(t)
	_, plain, err := store.CreateToken(t.Context(), newToken("agente", "bifrost", "dev"), "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	token, err := store.Find(t.Context(), plain)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if token.ExpiresAt != nil {
		t.Fatal("el token sin ttl venció")
	}
}

func TestAnAdminTokenIsMarkedAsOne(t *testing.T) {
	store := testStore(t)
	new := newToken("jimmy", "", "")
	new.Role = RoleAdmin
	token, plain, err := store.CreateToken(t.Context(), new, "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if token.Role != RoleAdmin {
		t.Fatal("el token no salió admin")
	}
	found, err := store.Find(t.Context(), plain)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if found.Role != RoleAdmin {
		t.Fatal("el token guardado no es admin")
	}
}

func TestTheTokenHashNeverLandsInClear(t *testing.T) {
	store := testStore(t)
	_, plain, err := store.CreateToken(t.Context(), newToken("agente", "bifrost", "dev"), "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	stored := storedText(t, store, "SELECT id || name || project || env || coalesce(keys::text, '') || hash FROM heimdall.tokens")
	if strings.Contains(stored, plain) {
		t.Fatal("el token quedó en claro en la base")
	}
}

func TestAUsedTokenSaysSo(t *testing.T) {
	store := testStore(t)
	token, _, err := store.CreateToken(t.Context(), newToken("agente", "bifrost", "dev"), "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	tokens, err := store.Tokens(t.Context())
	if err != nil {
		t.Fatalf("tokens: %v", err)
	}
	if tokens[0].LastUsed != nil {
		t.Fatal("el token nuevo ya tiene uso")
	}
	if err := store.Touch(t.Context(), token.ID); err != nil {
		t.Fatalf("touch: %v", err)
	}
	tokens, err = store.Tokens(t.Context())
	if err != nil {
		t.Fatalf("tokens: %v", err)
	}
	if tokens[0].LastUsed == nil {
		t.Fatal("el token usado no quedó marcado")
	}
}

func TestTheAuditRecordsNoValues(t *testing.T) {
	store := testStore(t)
	if err := store.Set(t.Context(), testOwner, "bifrost", "dev", "DB_PASSWORD", "hunter2", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	log, err := store.AuditLog(t.Context(), 10)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if len(log) != 1 {
		t.Fatalf("el audit tiene %d filas", len(log))
	}
	if log[0].Key == nil || *log[0].Key != "DB_PASSWORD" {
		t.Fatalf("el audit dice %v", log[0].Key)
	}
	if strings.Contains(storedText(t, store, "SELECT a::text FROM heimdall.audit a"), "hunter2") {
		t.Fatal("el audit guardó el valor")
	}
}

func TestTheAuditIsNewestFirstAndCapped(t *testing.T) {
	store := testStore(t)
	for _, name := range []string{"A", "B", "C"} {
		if err := store.Set(t.Context(), testOwner, "bifrost", "dev", name, "1", "berti"); err != nil {
			t.Fatalf("set: %v", err)
		}
	}
	log, err := store.AuditLog(t.Context(), 2)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if len(log) != 2 {
		t.Fatalf("devolvió %d filas", len(log))
	}
	if *log[0].Key != "C" || *log[1].Key != "B" {
		t.Fatalf("orden %v %v", *log[0].Key, *log[1].Key)
	}
}

func TestNamesListsWhatExists(t *testing.T) {
	store := testStore(t)
	for _, caso := range []struct{ project, env string }{
		{"bifrost", "dev"},
		{"bifrost", "prod"},
		{"axe", "dev"},
	} {
		if err := store.Set(t.Context(), testOwner, caso.project, caso.env, "A", "1", "berti"); err != nil {
			t.Fatalf("set: %v", err)
		}
	}
	want := []string{"axe/dev", "bifrost/dev", "bifrost/prod"}
	if got := names(t, store); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestAnEnvironmentCanExistBeforeItsFirstSecret(t *testing.T) {
	store := testStore(t)
	if err := store.CreateEnvironment(t.Context(), testOwner, "bifrost", "dev", "berti"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := names(t, store); !reflect.DeepEqual(got, []string{"bifrost/dev"}) {
		t.Fatalf("got %v", got)
	}
	secrets, err := store.Secrets(t.Context(), testOwner, "bifrost", "dev")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if len(secrets) != 0 {
		t.Fatalf("el entorno vacío tiene %v", secrets)
	}
}

func TestTheListingAlsoShowsWhatWasThereBeforeTheTable(t *testing.T) {
	store := testStore(t)
	if err := store.Set(t.Context(), testOwner, "axe", "dev", "A", "1", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.CreateEnvironment(t.Context(), testOwner, "bifrost", "dev", "berti"); err != nil {
		t.Fatalf("create: %v", err)
	}
	want := []string{"axe/dev", "bifrost/dev"}
	if got := names(t, store); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestDroppingAnEnvironmentTakesItsSecrets(t *testing.T) {
	store := testStore(t)
	if err := store.Set(t.Context(), testOwner, "bifrost", "dev", "A", "1", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.Set(t.Context(), testOwner, "bifrost", "prod", "A", "2", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.DropEnvironment(t.Context(), testOwner, "bifrost", "dev", "berti"); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if got := names(t, store); !reflect.DeepEqual(got, []string{"bifrost/prod"}) {
		t.Fatalf("got %v", got)
	}
	secrets, err := store.Secrets(t.Context(), testOwner, "bifrost", "dev")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if len(secrets) != 0 {
		t.Fatalf("quedó %v", secrets)
	}
	kept, err := store.Secrets(t.Context(), testOwner, "bifrost", "prod")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if kept["A"] != "2" {
		t.Fatalf("prod quedó en %q", kept["A"])
	}
}

func TestDroppingAnEmptyEnvironmentWorksToo(t *testing.T) {
	store := testStore(t)
	if err := store.CreateEnvironment(t.Context(), testOwner, "bifrost", "dev", "berti"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.DropEnvironment(t.Context(), testOwner, "bifrost", "dev", "berti"); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if got := names(t, store); len(got) != 0 {
		t.Fatalf("quedó %v", got)
	}
	if err := store.DropEnvironment(t.Context(), testOwner, "bifrost", "dev", "berti"); err == nil {
		t.Fatal("sacó un entorno que ya no estaba")
	}
}

func TestDroppingAProjectTakesEverything(t *testing.T) {
	store := testStore(t)
	for _, caso := range []struct{ project, env, value string }{
		{"bifrost", "dev", "1"},
		{"bifrost", "prod", "2"},
		{"axe", "dev", "3"},
	} {
		if err := store.Set(t.Context(), testOwner, caso.project, caso.env, "A", caso.value, "berti"); err != nil {
			t.Fatalf("set: %v", err)
		}
	}
	if err := store.DropProject(t.Context(), testOwner, "bifrost", "berti"); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if got := names(t, store); !reflect.DeepEqual(got, []string{"axe/dev"}) {
		t.Fatalf("got %v", got)
	}
	if err := store.DropProject(t.Context(), testOwner, "bifrost", "berti"); err == nil {
		t.Fatal("sacó un proyecto que ya no estaba")
	}
}

func TestDroppingAnEnvironmentTakesItsTokens(t *testing.T) {
	store := testStore(t)
	if err := store.Set(t.Context(), testOwner, "bifrost", "dev", "A", "1", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	_, plain, err := store.CreateToken(t.Context(), newToken("agente", "bifrost", "dev"), "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if err := store.DropEnvironment(t.Context(), testOwner, "bifrost", "dev", "berti"); err != nil {
		t.Fatalf("drop: %v", err)
	}
	found, err := store.Find(t.Context(), plain)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if found != nil {
		t.Fatal("el token sobrevivió al entorno")
	}
	tokens, err := store.Tokens(t.Context())
	if err != nil {
		t.Fatalf("tokens: %v", err)
	}
	if len(tokens) != 0 {
		t.Fatalf("quedaron %d tokens", len(tokens))
	}
}

func TestDroppingAProjectLeavesTheAdminTokensAlone(t *testing.T) {
	store := testStore(t)
	if err := store.Set(t.Context(), testOwner, "bifrost", "dev", "A", "1", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	_, agente, err := store.CreateToken(t.Context(), newToken("agente", "bifrost", "dev"), "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	admin := newToken("jimmy", "", "")
	admin.Role = RoleAdmin
	_, plain, err := store.CreateToken(t.Context(), admin, "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if err := store.DropProject(t.Context(), testOwner, "bifrost", "berti"); err != nil {
		t.Fatalf("drop: %v", err)
	}
	found, err := store.Find(t.Context(), plain)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if found == nil {
		t.Fatal("se llevó el token de administración")
	}
	found, err = store.Find(t.Context(), agente)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if found != nil {
		t.Fatal("el token del proyecto sobrevivió")
	}
}

func TestRenamingAnEnvironmentKeepsItsValuesWorking(t *testing.T) {
	store := testStore(t)
	if err := store.Set(t.Context(), testOwner, "bifrost", "dev", "STRIPE_KEY", "sk_test_123", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.RenameEnvironment(t.Context(), testOwner, "bifrost", "dev", "testing", "berti"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if got := names(t, store); !reflect.DeepEqual(got, []string{"bifrost/testing"}) {
		t.Fatalf("got %v", got)
	}
	secrets, err := store.Secrets(t.Context(), testOwner, "bifrost", "testing")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if secrets["STRIPE_KEY"] != "sk_test_123" {
		t.Fatalf("quedó en %q", secrets["STRIPE_KEY"])
	}
	old, err := store.Secrets(t.Context(), testOwner, "bifrost", "dev")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if len(old) != 0 {
		t.Fatalf("el nombre viejo tiene %v", old)
	}
}

func TestRenamingAnEnvironmentIntoATakenOneIsRefused(t *testing.T) {
	store := testStore(t)
	if err := store.Set(t.Context(), testOwner, "bifrost", "dev", "A", "1", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.Set(t.Context(), testOwner, "bifrost", "prod", "A", "2", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.RenameEnvironment(t.Context(), testOwner, "bifrost", "dev", "prod", "berti"); err == nil {
		t.Fatal("pisó un entorno que ya existía")
	}
	secrets, err := store.Secrets(t.Context(), testOwner, "bifrost", "prod")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if secrets["A"] != "2" {
		t.Fatalf("prod quedó en %q", secrets["A"])
	}
}

func TestRenamingAProjectKeepsEveryValueWorking(t *testing.T) {
	store := testStore(t)
	if err := store.Set(t.Context(), testOwner, "bifrost", "dev", "A", "1", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.Set(t.Context(), testOwner, "bifrost", "prod", "B", "2", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.RenameProject(t.Context(), testOwner, "bifrost", "puente", "berti"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	want := []string{"puente/dev", "puente/prod"}
	if got := names(t, store); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	dev, err := store.Secrets(t.Context(), testOwner, "puente", "dev")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	prod, err := store.Secrets(t.Context(), testOwner, "puente", "prod")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if dev["A"] != "1" || prod["B"] != "2" {
		t.Fatalf("quedó %v %v", dev, prod)
	}
}

func TestRenamingAProjectOntoAnotherIsRefused(t *testing.T) {
	store := testStore(t)
	if err := store.Set(t.Context(), testOwner, "bifrost", "dev", "A", "1", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.Set(t.Context(), testOwner, "axe", "dev", "A", "2", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.RenameProject(t.Context(), testOwner, "bifrost", "axe", "berti"); err == nil {
		t.Fatal("pisó un proyecto que ya existía")
	}
	secrets, err := store.Secrets(t.Context(), testOwner, "axe", "dev")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if secrets["A"] != "2" {
		t.Fatalf("axe quedó en %q", secrets["A"])
	}
}

func TestRenamingAnEmptyProjectIsRefused(t *testing.T) {
	if err := testStore(t).RenameProject(t.Context(), testOwner, "nada", "otro", "berti"); err == nil {
		t.Fatal("renombró un proyecto que no existe")
	}
}

func TestTheWildcardIsAllowedInATokenScope(t *testing.T) {
	store := testStore(t)
	_, plain, err := store.CreateToken(t.Context(), NewToken{Owner: testOwner, Name: "jimmy", Project: "*", Env: "dev"}, "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	token, err := store.Find(t.Context(), plain)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if token.Project != "*" || token.Env != "dev" {
		t.Fatalf("quedó %s/%s", token.Project, token.Env)
	}
	if _, _, err := store.CreateToken(t.Context(), NewToken{Owner: testOwner, Name: "jimmy", Project: "*", Env: "*"}, "berti"); err != nil {
		t.Fatalf("token: %v", err)
	}
}

func TestTwoOwnersDoNotSeeTheSameProject(t *testing.T) {
	store := testStore(t)
	other := Org("otra")
	if err := store.Set(t.Context(), testOwner, "bifrost", "dev", "STRIPE_KEY", "sk_test_123", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.Set(t.Context(), other, "bifrost", "dev", "STRIPE_KEY", "sk_test_999", "otro"); err != nil {
		t.Fatalf("set: %v", err)
	}

	mine, err := store.Secrets(t.Context(), testOwner, "bifrost", "dev")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	theirs, err := store.Secrets(t.Context(), other, "bifrost", "dev")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if mine["STRIPE_KEY"] != "sk_test_123" || theirs["STRIPE_KEY"] != "sk_test_999" {
		t.Fatalf("se cruzaron: %v y %v", mine, theirs)
	}

	names, err := store.Names(t.Context(), other)
	if err != nil {
		t.Fatalf("names: %v", err)
	}
	if len(names) != 1 || names[0] != "bifrost/dev" {
		t.Fatalf("los nombres del otro quedaron %v", names)
	}

	if err := store.DropProject(t.Context(), other, "bifrost", "otro"); err != nil {
		t.Fatalf("drop: %v", err)
	}
	kept, err := store.Secrets(t.Context(), testOwner, "bifrost", "dev")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if kept["STRIPE_KEY"] != "sk_test_123" {
		t.Fatalf("el drop del otro se llevó lo mío: %v", kept)
	}
}
