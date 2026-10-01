package heimdall

import (
	"strings"
	"testing"
)

func mint(t *testing.T, store *Store, new NewToken) (Token, string) {
	t.Helper()
	token, plain, err := store.CreateToken(t.Context(), new, "berti")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	return token, plain
}

func refusalOf(t *testing.T, plain string, store *Store) Actor {
	t.Helper()
	actor, refusal := Authenticate(t.Context(), store, "", plain, "hd_admin")
	if refusal != nil {
		t.Fatalf("no autenticó: %d %s", refusal.Status, refusal.Message)
	}
	return actor
}

func TestARequestWithoutATokenIsRejected(t *testing.T) {
	_, refusal := Authenticate(t.Context(), testStore(t), "", "", "hd_admin")
	if refusal == nil {
		t.Fatal("entró sin token")
	}
	if refusal.Status != 401 || refusal.Message != "falta el token" {
		t.Fatalf("dio %d %s", refusal.Status, refusal.Message)
	}
}

func TestTheAdminWritesAndATokenReads(t *testing.T) {
	store := testStore(t)
	if err := store.Set(t.Context(), "bifrost", "dev", "A", "1", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	_, plain := mint(t, store, NewToken{Name: "agente", Project: "bifrost", Env: "dev"})
	actor := refusalOf(t, plain, store)
	if actor.Admin || actor.Name != "agente" || actor.Token == nil {
		t.Fatalf("quedó %v", actor)
	}
	if refusal := actor.ScopedRequest("bifrost", "dev"); refusal != nil {
		t.Fatalf("no llegó: %d %s", refusal.Status, refusal.Message)
	}
	secrets, err := store.Secrets(t.Context(), "bifrost", "dev")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	visible := actor.Visible(secrets)
	if len(visible) != 1 || visible["A"] != "1" {
		t.Fatalf("vio %v", visible)
	}

	admin, refusal := Authenticate(t.Context(), store, "", "hd_admin", "hd_admin")
	if refusal != nil {
		t.Fatalf("no entró el admin: %d %s", refusal.Status, refusal.Message)
	}
	if !admin.Admin || admin.Name != "admin" {
		t.Fatalf("el admin quedó %v", admin)
	}
	if len(admin.Visible(secrets)) != 1 {
		t.Fatal("el admin no ve todo")
	}
}

func TestATokenDoesNotReachAnotherEnvironment(t *testing.T) {
	store := testStore(t)
	_, plain := mint(t, store, NewToken{Name: "agente", Project: "bifrost", Env: "dev"})
	actor := refusalOf(t, plain, store)
	refusal := actor.Allows("bifrost", "prod")
	if refusal == nil {
		t.Fatal("llegó a producción")
	}
	if refusal.Status != 403 || !strings.Contains(refusal.Message, "no llega") {
		t.Fatalf("dio %d %s", refusal.Status, refusal.Message)
	}
}

func TestATokenWithKeysOnlySeesThose(t *testing.T) {
	store := testStore(t)
	if err := store.Set(t.Context(), "bifrost", "dev", "A", "1", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.Set(t.Context(), "bifrost", "dev", "B", "2", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	_, plain := mint(t, store, NewToken{Name: "agente", Project: "bifrost", Env: "dev", Keys: []string{"A"}})
	actor := refusalOf(t, plain, store)
	secrets, err := store.Secrets(t.Context(), "bifrost", "dev")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	visible := actor.Visible(secrets)
	if len(visible) != 1 || visible["A"] != "1" {
		t.Fatalf("vio %v", visible)
	}
	if refusal := actor.AllowsKey("A"); refusal != nil {
		t.Fatalf("no vio A: %s", refusal.Message)
	}
	refusal := actor.AllowsKey("B")
	if refusal == nil {
		t.Fatal("vio B")
	}
	if refusal.Status != 403 || refusal.Message != "este token no ve B" {
		t.Fatalf("dio %d %s", refusal.Status, refusal.Message)
	}
	if refusal := actor.AllowsKey(""); refusal != nil {
		t.Fatalf("se quejó sin pedir ninguna clave: %s", refusal.Message)
	}
}

func TestATokenWithoutKeysSeesTheWholeEnvironment(t *testing.T) {
	store := testStore(t)
	if err := store.Set(t.Context(), "bifrost", "dev", "A", "1", "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	_, plain := mint(t, store, NewToken{Name: "agente", Project: "bifrost", Env: "dev"})
	actor := refusalOf(t, plain, store)
	secrets, err := store.Secrets(t.Context(), "bifrost", "dev")
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if len(actor.Visible(secrets)) != 1 {
		t.Fatalf("vio %v", actor.Visible(secrets))
	}
	if refusal := actor.AllowsKey("LO_QUE_SEA"); refusal != nil {
		t.Fatalf("le puso un tope: %s", refusal.Message)
	}
}

func TestATokenCannotWriteOrAdministrate(t *testing.T) {
	store := testStore(t)
	_, plain := mint(t, store, NewToken{Name: "agente", Project: "bifrost", Env: "dev"})
	actor := refusalOf(t, plain, store)
	refusal := actor.AllowsAdmin()
	if refusal == nil {
		t.Fatal("un token cualquiera administró")
	}
	if refusal.Status != 403 || refusal.Message != "este token no administra" {
		t.Fatalf("dio %d %s", refusal.Status, refusal.Message)
	}
}

func TestAnAdminTokenAdministrates(t *testing.T) {
	store := testStore(t)
	_, plain := mint(t, store, NewToken{Name: "jimmy", Role: RoleAdmin})
	actor := refusalOf(t, plain, store)
	if !actor.Admin || actor.Name != "jimmy" || actor.Token != nil {
		t.Fatalf("quedó %v", actor)
	}
	if refusal := actor.AllowsAdmin(); refusal != nil {
		t.Fatalf("no pudo administrar: %s", refusal.Message)
	}
	if refusal := actor.Allows("axe", "prod"); refusal != nil {
		t.Fatalf("no llegó: %s", refusal.Message)
	}
}

func TestAnExpiredTokenIsA401(t *testing.T) {
	store := testStore(t)
	expired := int64(-1)
	_, plain := mint(t, store, NewToken{Name: "agente", Project: "bifrost", Env: "dev", TTL: &expired})
	_, refusal := Authenticate(t.Context(), store, "", plain, "hd_admin")
	if refusal == nil {
		t.Fatal("el token vencido entró")
	}
	if refusal.Status != 401 || refusal.Message != "ese token venció" {
		t.Fatalf("dio %d %s", refusal.Status, refusal.Message)
	}
}

func TestABadTokenIsA401(t *testing.T) {
	_, refusal := Authenticate(t.Context(), testStore(t), "", "hd_nada", "hd_admin")
	if refusal == nil {
		t.Fatal("entró con un token inventado")
	}
	if refusal.Status != 401 || refusal.Message != "ese token no sirve" {
		t.Fatalf("dio %d %s", refusal.Status, refusal.Message)
	}
}

func TestTheUserTheAppResolvedEnters(t *testing.T) {
	store := testStore(t)
	actor, refusal := Authenticate(t.Context(), store, "berti", "", "hd_admin")
	if refusal != nil {
		t.Fatalf("no entró: %d %s", refusal.Status, refusal.Message)
	}
	if !actor.Admin || actor.Name != "berti" {
		t.Fatalf("quedó %v", actor)
	}
	admin, refusal := Authenticate(t.Context(), store, "cualquiera", "hd_admin", "hd_admin")
	if refusal != nil || !admin.Admin {
		t.Fatalf("un usuario cualquiera no dejó pasar al token: %v", refusal)
	}
}

func TestAUsedTokenIsMarkedAsUsed(t *testing.T) {
	store := testStore(t)
	_, plain := mint(t, store, NewToken{Name: "agente", Project: "bifrost", Env: "dev"})
	refusalOf(t, plain, store)
	tokens, err := store.Tokens(t.Context())
	if err != nil {
		t.Fatalf("tokens: %v", err)
	}
	if tokens[0].LastUsed == nil {
		t.Fatal("el token usado no quedó marcado")
	}
}

func TestAWildcardTokenReachesEveryProjectInItsEnvironment(t *testing.T) {
	store := testStore(t)
	_, plain := mint(t, store, NewToken{Name: "runner", Project: "*", Env: "dev"})
	actor := refusalOf(t, plain, store)
	for _, project := range []string{"bifrost", "axe", "picsel"} {
		if refusal := actor.Allows(project, "dev"); refusal != nil {
			t.Fatalf("no llegó a %s/dev: %s", project, refusal.Message)
		}
	}
	if refusal := actor.Allows("bifrost", "prod"); refusal == nil {
		t.Fatal("el comodín llegó a producción")
	}
}

func TestAWildcardInTheEnvironmentStaysInsideItsProject(t *testing.T) {
	store := testStore(t)
	_, plain := mint(t, store, NewToken{Name: "agente", Project: "bifrost", Env: "*"})
	actor := refusalOf(t, plain, store)
	if refusal := actor.Allows("bifrost", "prod"); refusal != nil {
		t.Fatalf("no llegó a bifrost/prod: %s", refusal.Message)
	}
	if refusal := actor.Allows("axe", "dev"); refusal == nil {
		t.Fatal("se fue a otro proyecto")
	}
}

func TestScopedRequestRejectsAnEmptyScope(t *testing.T) {
	actor, refusal := Authenticate(t.Context(), testStore(t), "", "hd_admin", "hd_admin")
	if refusal != nil {
		t.Fatalf("no entró el admin: %s", refusal.Message)
	}
	refusal = actor.ScopedRequest("", "dev")
	if refusal == nil {
		t.Fatal("leyó sin proyecto")
	}
	if refusal.Status != 400 || refusal.Message != "faltan project y env" {
		t.Fatalf("dio %d %s", refusal.Status, refusal.Message)
	}
	if refusal := actor.ScopedRequest("bifrost", "dev"); refusal != nil {
		t.Fatalf("el admin no pudo leer: %s", refusal.Message)
	}
}

func TestRefuseMapsWhatTheStoreReturned(t *testing.T) {
	refusal := Refuse(bad("«A B» no es un nombre de variable válido"))
	if refusal.Status != 400 || !strings.Contains(refusal.Message, "no es un nombre") {
		t.Fatalf("dio %d %s", refusal.Status, refusal.Message)
	}
	refusal = Refuse(internal("disco lleno"))
	if refusal.Status != 500 || refusal.Message != "algo se rompió acá adentro" {
		t.Fatalf("dio %d %s", refusal.Status, refusal.Message)
	}
	if refusal.Cause == nil || refusal.Cause.Error() != "disco lleno" {
		t.Fatalf("perdió la causa: %v", refusal.Cause)
	}
}
