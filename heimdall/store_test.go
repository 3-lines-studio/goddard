package heimdall

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	return storeAt(t, filepath.Join(t.TempDir(), "heimdall.db"))
}

func storeAt(t *testing.T, path string) *Store {
	t.Helper()
	store, err := OpenStore(path, testKey(t))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func newToken(name, project, env string) NewToken {
	return NewToken{Name: name, Project: project, Env: env}
}

func agent(t *testing.T, store *Store) string {
	t.Helper()
	_, plain, err := store.CreateToken(newToken("agente", "bifrost", "dev"), "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	return plain
}

func dump(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("dump: %v", err)
	}
	if wal, err := os.ReadFile(path + "-wal"); err == nil {
		raw = append(raw, wal...)
	}
	return raw
}

func names(t *testing.T, store *Store) []string {
	t.Helper()
	got, err := store.Names()
	if err != nil {
		t.Fatalf("names: %v", err)
	}
	return got
}

func TestASecretSurvivesAReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "heimdall.db")
	storeAt(t, path).Set("bifrost", "dev", "STRIPE_KEY", "sk_test_123", "berti")
	reopened := storeAt(t, path)
	secrets, err := reopened.Secrets("bifrost", "dev")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if secrets["STRIPE_KEY"] != "sk_test_123" {
		t.Fatalf("leyó %q", secrets["STRIPE_KEY"])
	}
}

func TestTheValueNeverLandsInClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "heimdall.db")
	store := storeAt(t, path)
	if err := store.Set("bifrost", "dev", "DB_PASSWORD", "hunter2", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if bytes.Contains(dump(t, path), []byte("hunter2")) {
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
		if err := store.Set(caso.project, caso.env, "A", caso.value, "berti"); err != nil {
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
		secrets, err := store.Secrets(caso.project, caso.env)
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
		if err := store.Set(caso.project, caso.env, caso.name, "1", "berti"); err == nil {
			t.Fatalf("aceptó %s/%s/%s", caso.project, caso.env, caso.name)
		}
	}
}

func TestMissingSecretsAreAnEmptyMap(t *testing.T) {
	secrets, err := testStore(t).Secrets("bifrost", "prod")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if len(secrets) != 0 {
		t.Fatalf("devolvió %v", secrets)
	}
}

func TestARemovedKeyIsNoLongerThere(t *testing.T) {
	store := testStore(t)
	if err := store.Set("bifrost", "dev", "A", "1", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.Unset("bifrost", "dev", "A", "berti"); err != nil {
		t.Fatalf("unset: %v", err)
	}
	secrets, err := store.Secrets("bifrost", "dev")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if len(secrets) != 0 {
		t.Fatalf("quedó %v", secrets)
	}
	if err := store.Unset("bifrost", "dev", "A", "berti"); err == nil {
		t.Fatal("sacó dos veces la misma clave")
	}
}

func TestAFailedUnsetLeavesNoAuditRow(t *testing.T) {
	store := testStore(t)
	if err := store.Unset("bifrost", "dev", "A", "berti"); err == nil {
		t.Fatal("sacó algo que no estaba")
	}
	log, err := store.AuditLog(10)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if len(log) != 0 {
		t.Fatalf("dejó %d filas", len(log))
	}
}

func TestOnlyTheMatchingTokenIsFound(t *testing.T) {
	store := testStore(t)
	token, plain, err := store.CreateToken(newToken("agente", "bifrost", "dev"), "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	found, err := store.Find("hd_nada")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if found != nil {
		t.Fatal("encontró un token que no existe")
	}
	found, err = store.Find(plain)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if found == nil || found.ID != token.ID {
		t.Fatal("no encontró el token")
	}
}

func TestARevokedTokenIsGone(t *testing.T) {
	store := testStore(t)
	token, plain, err := store.CreateToken(newToken("agente", "bifrost", "dev"), "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if _, err := store.Revoke(token.ID, "berti"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	found, err := store.Find(plain)
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
	_, plain, err := store.CreateToken(new, "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if _, err := store.Find(plain); err == nil {
		t.Fatal("el token vencido entró")
	}
}

func TestATokenWithoutTTLDoesNotExpire(t *testing.T) {
	store := testStore(t)
	_, plain, err := store.CreateToken(newToken("agente", "bifrost", "dev"), "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	token, err := store.Find(plain)
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
	new.Admin = true
	token, plain, err := store.CreateToken(new, "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if !token.Admin {
		t.Fatal("el token no salió admin")
	}
	found, err := store.Find(plain)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if !found.Admin {
		t.Fatal("el token guardado no es admin")
	}
}

func TestTheTokenHashNeverLandsInClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "heimdall.db")
	store := storeAt(t, path)
	_, plain, err := store.CreateToken(newToken("agente", "bifrost", "dev"), "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if bytes.Contains(dump(t, path), []byte(plain)) {
		t.Fatal("el token quedó en claro en la base")
	}
}

func TestAUsedTokenSaysSo(t *testing.T) {
	store := testStore(t)
	token, _, err := store.CreateToken(newToken("agente", "bifrost", "dev"), "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	tokens, err := store.Tokens()
	if err != nil {
		t.Fatalf("tokens: %v", err)
	}
	if tokens[0].LastUsed != nil {
		t.Fatal("el token nuevo ya tiene uso")
	}
	if err := store.Touch(token.ID); err != nil {
		t.Fatalf("touch: %v", err)
	}
	tokens, err = store.Tokens()
	if err != nil {
		t.Fatalf("tokens: %v", err)
	}
	if tokens[0].LastUsed == nil {
		t.Fatal("el token usado no quedó marcado")
	}
}

func TestTheAuditRecordsNoValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "heimdall.db")
	store := storeAt(t, path)
	if err := store.Set("bifrost", "dev", "DB_PASSWORD", "hunter2", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	log, err := store.AuditLog(10)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if len(log) != 1 {
		t.Fatalf("el audit tiene %d filas", len(log))
	}
	if log[0].Key == nil || *log[0].Key != "DB_PASSWORD" {
		t.Fatalf("el audit dice %v", log[0].Key)
	}
	if bytes.Contains(dump(t, path), []byte("hunter2")) {
		t.Fatal("el audit guardó el valor")
	}
}

func TestTheAuditIsNewestFirstAndCapped(t *testing.T) {
	store := testStore(t)
	for _, name := range []string{"A", "B", "C"} {
		if err := store.Set("bifrost", "dev", name, "1", "berti"); err != nil {
			t.Fatalf("set: %v", err)
		}
	}
	log, err := store.AuditLog(2)
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
		if err := store.Set(caso.project, caso.env, "A", "1", "berti"); err != nil {
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
	if err := store.CreateEnvironment("bifrost", "dev", "berti"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := names(t, store); !reflect.DeepEqual(got, []string{"bifrost/dev"}) {
		t.Fatalf("got %v", got)
	}
	secrets, err := store.Secrets("bifrost", "dev")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if len(secrets) != 0 {
		t.Fatalf("el entorno vacío tiene %v", secrets)
	}
}

func TestTheListingAlsoShowsWhatWasThereBeforeTheTable(t *testing.T) {
	store := testStore(t)
	if err := store.Set("axe", "dev", "A", "1", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.CreateEnvironment("bifrost", "dev", "berti"); err != nil {
		t.Fatalf("create: %v", err)
	}
	want := []string{"axe/dev", "bifrost/dev"}
	if got := names(t, store); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestDroppingAnEnvironmentTakesItsSecrets(t *testing.T) {
	store := testStore(t)
	if err := store.Set("bifrost", "dev", "A", "1", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.Set("bifrost", "prod", "A", "2", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.DropEnvironment("bifrost", "dev", "berti"); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if got := names(t, store); !reflect.DeepEqual(got, []string{"bifrost/prod"}) {
		t.Fatalf("got %v", got)
	}
	secrets, err := store.Secrets("bifrost", "dev")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if len(secrets) != 0 {
		t.Fatalf("quedó %v", secrets)
	}
	kept, err := store.Secrets("bifrost", "prod")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if kept["A"] != "2" {
		t.Fatalf("prod quedó en %q", kept["A"])
	}
}

func TestDroppingAnEmptyEnvironmentWorksToo(t *testing.T) {
	store := testStore(t)
	if err := store.CreateEnvironment("bifrost", "dev", "berti"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.DropEnvironment("bifrost", "dev", "berti"); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if got := names(t, store); len(got) != 0 {
		t.Fatalf("quedó %v", got)
	}
	if err := store.DropEnvironment("bifrost", "dev", "berti"); err == nil {
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
		if err := store.Set(caso.project, caso.env, "A", caso.value, "berti"); err != nil {
			t.Fatalf("set: %v", err)
		}
	}
	if err := store.DropProject("bifrost", "berti"); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if got := names(t, store); !reflect.DeepEqual(got, []string{"axe/dev"}) {
		t.Fatalf("got %v", got)
	}
	if err := store.DropProject("bifrost", "berti"); err == nil {
		t.Fatal("sacó un proyecto que ya no estaba")
	}
}

func TestDroppingAnEnvironmentTakesItsTokens(t *testing.T) {
	store := testStore(t)
	if err := store.Set("bifrost", "dev", "A", "1", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	_, plain, err := store.CreateToken(newToken("agente", "bifrost", "dev"), "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if err := store.DropEnvironment("bifrost", "dev", "berti"); err != nil {
		t.Fatalf("drop: %v", err)
	}
	found, err := store.Find(plain)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if found != nil {
		t.Fatal("el token sobrevivió al entorno")
	}
	tokens, err := store.Tokens()
	if err != nil {
		t.Fatalf("tokens: %v", err)
	}
	if len(tokens) != 0 {
		t.Fatalf("quedaron %d tokens", len(tokens))
	}
}

func TestDroppingAProjectLeavesTheAdminTokensAlone(t *testing.T) {
	store := testStore(t)
	if err := store.Set("bifrost", "dev", "A", "1", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	_, agente, err := store.CreateToken(newToken("agente", "bifrost", "dev"), "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	admin := newToken("jimmy", "", "")
	admin.Admin = true
	_, plain, err := store.CreateToken(admin, "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if err := store.DropProject("bifrost", "berti"); err != nil {
		t.Fatalf("drop: %v", err)
	}
	found, err := store.Find(plain)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if found == nil {
		t.Fatal("se llevó el token de administración")
	}
	found, err = store.Find(agente)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if found != nil {
		t.Fatal("el token del proyecto sobrevivió")
	}
}

func TestRenamingAnEnvironmentKeepsItsValuesWorking(t *testing.T) {
	store := testStore(t)
	if err := store.Set("bifrost", "dev", "STRIPE_KEY", "sk_test_123", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.RenameEnvironment("bifrost", "dev", "testing", "berti"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if got := names(t, store); !reflect.DeepEqual(got, []string{"bifrost/testing"}) {
		t.Fatalf("got %v", got)
	}
	secrets, err := store.Secrets("bifrost", "testing")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if secrets["STRIPE_KEY"] != "sk_test_123" {
		t.Fatalf("quedó en %q", secrets["STRIPE_KEY"])
	}
	old, err := store.Secrets("bifrost", "dev")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if len(old) != 0 {
		t.Fatalf("el nombre viejo tiene %v", old)
	}
}

func TestRenamingAnEnvironmentIntoATakenOneIsRefused(t *testing.T) {
	store := testStore(t)
	if err := store.Set("bifrost", "dev", "A", "1", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.Set("bifrost", "prod", "A", "2", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.RenameEnvironment("bifrost", "dev", "prod", "berti"); err == nil {
		t.Fatal("pisó un entorno que ya existía")
	}
	secrets, err := store.Secrets("bifrost", "prod")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if secrets["A"] != "2" {
		t.Fatalf("prod quedó en %q", secrets["A"])
	}
}

func TestRenamingAProjectKeepsEveryValueWorking(t *testing.T) {
	store := testStore(t)
	if err := store.Set("bifrost", "dev", "A", "1", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.Set("bifrost", "prod", "B", "2", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.RenameProject("bifrost", "puente", "berti"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	want := []string{"puente/dev", "puente/prod"}
	if got := names(t, store); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	dev, err := store.Secrets("puente", "dev")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	prod, err := store.Secrets("puente", "prod")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if dev["A"] != "1" || prod["B"] != "2" {
		t.Fatalf("quedó %v %v", dev, prod)
	}
}

func TestRenamingAProjectOntoAnotherIsRefused(t *testing.T) {
	store := testStore(t)
	if err := store.Set("bifrost", "dev", "A", "1", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.Set("axe", "dev", "A", "2", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.RenameProject("bifrost", "axe", "berti"); err == nil {
		t.Fatal("pisó un proyecto que ya existía")
	}
	secrets, err := store.Secrets("axe", "dev")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if secrets["A"] != "2" {
		t.Fatalf("axe quedó en %q", secrets["A"])
	}
}

func TestRenamingAnEmptyProjectIsRefused(t *testing.T) {
	if err := testStore(t).RenameProject("nada", "otro", "berti"); err == nil {
		t.Fatal("renombró un proyecto que no existe")
	}
}

func TestALoginLinkWorksOnce(t *testing.T) {
	store := testStore(t)
	link, err := store.CreateLogin("berti@ejemplo.com", 900)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	email, err := store.ConsumeLogin(link)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if email != "berti@ejemplo.com" {
		t.Fatalf("dio %q", email)
	}
	if _, err := store.ConsumeLogin(link); err == nil {
		t.Fatal("el link sirvió dos veces")
	}
}

func TestAnExpiredLoginLinkIsRefused(t *testing.T) {
	store := testStore(t)
	link, err := store.CreateLogin("berti@ejemplo.com", -1)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, err := store.ConsumeLogin(link); err == nil {
		t.Fatal("el link vencido sirvió")
	}
}

func TestASecondLinkIsACooldownTooSSoon(t *testing.T) {
	store := testStore(t)
	asked, err := store.AskedRecently("berti@ejemplo.com", 60)
	if err != nil {
		t.Fatalf("asked: %v", err)
	}
	if asked {
		t.Fatal("preguntó antes de pedir nada")
	}
	if _, err := store.CreateLogin("berti@ejemplo.com", 900); err != nil {
		t.Fatalf("login: %v", err)
	}
	asked, err = store.AskedRecently("berti@ejemplo.com", 60)
	if err != nil {
		t.Fatalf("asked: %v", err)
	}
	if !asked {
		t.Fatal("no vio el link recién pedido")
	}
	asked, err = store.AskedRecently("otro@ejemplo.com", 60)
	if err != nil {
		t.Fatalf("asked: %v", err)
	}
	if asked {
		t.Fatal("le pegó el cooldown de otra dirección")
	}
}

func TestASessionLivesAndDies(t *testing.T) {
	store := testStore(t)
	cookie, err := store.CreateSession("berti@ejemplo.com", 60)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	email, ok, err := store.Session(cookie)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if !ok || email != "berti@ejemplo.com" {
		t.Fatalf("la sesión dio %q %v", email, ok)
	}
	if err := store.DropSession(cookie); err != nil {
		t.Fatalf("drop: %v", err)
	}
	_, ok, err = store.Session(cookie)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if ok {
		t.Fatal("la sesión sobrevivió al logout")
	}
}

func TestAnExpiredSessionIsNoSession(t *testing.T) {
	store := testStore(t)
	cookie, err := store.CreateSession("berti@ejemplo.com", -1)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	_, ok, err := store.Session(cookie)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if ok {
		t.Fatal("la sesión vencida entró")
	}
}

func TestSweepingClearsWhatExpired(t *testing.T) {
	store := testStore(t)
	login, err := store.CreateLogin("berti@ejemplo.com", -1)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	dead, err := store.CreateSession("berti@ejemplo.com", -1)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	live, err := store.CreateSession("berti@ejemplo.com", 60)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if err := store.Sweep(); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if _, err := store.ConsumeLogin(login); err == nil {
		t.Fatal("el link vencido sobrevivió al barrido")
	}
	_, ok, err := store.Session(dead)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if ok {
		t.Fatal("la sesión vencida sobrevivió al barrido")
	}
	_, ok, err = store.Session(live)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if !ok {
		t.Fatal("el barrido se llevó una sesión viva")
	}
}

func TestNeitherTheLoginNorTheSessionLandsInClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "heimdall.db")
	store := storeAt(t, path)
	login, err := store.CreateLogin("berti@ejemplo.com", 900)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	session, err := store.CreateSession("berti@ejemplo.com", 900)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	raw := dump(t, path)
	if bytes.Contains(raw, []byte(login)) {
		t.Fatal("el link quedó en claro")
	}
	if bytes.Contains(raw, []byte(session)) {
		t.Fatal("la sesión quedó en claro")
	}
}

func TestTheWildcardIsAllowedInATokenScope(t *testing.T) {
	store := testStore(t)
	_, plain, err := store.CreateToken(NewToken{Name: "jimmy", Project: "*", Env: "dev"}, "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	token, err := store.Find(plain)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if token.Project != "*" || token.Env != "dev" {
		t.Fatalf("quedó %s/%s", token.Project, token.Env)
	}
	if _, _, err := store.CreateToken(NewToken{Name: "jimmy", Project: "*", Env: "*"}, "berti"); err != nil {
		t.Fatalf("token: %v", err)
	}
}
