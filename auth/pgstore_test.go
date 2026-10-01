package auth

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/3-lines-studio/goddard/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const testLock = 0x676f6464544553

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
	if _, err := db.ExecContext(t.Context(), "DROP SCHEMA IF EXISTS goddard, auth, chat, heimdall, axe, skill, memo, schedule CASCADE; DROP TABLE IF EXISTS public.schema_migrations"); err != nil {
		t.Fatalf("no pude limpiar: %v", err)
	}
	if _, err := migrations.Apply(t.Context(), db); err != nil {
		t.Fatalf("no pude migrar: %v", err)
	}
	return db
}

func testStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(testDB(t))
}

func signIn(t *testing.T, store *Store) User {
	t.Helper()
	link, err := store.CreateLogin(t.Context(), "berti@ejemplo.com", LoginTTL)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	user, err := store.ConsumeLogin(t.Context(), link)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	return user
}

func TestTheFirstLoginCreatesTheUserAndTheSecondFindsIt(t *testing.T) {
	store := testStore(t)
	user := signIn(t, store)
	if user.Email != "berti@ejemplo.com" || user.Name != "berti" || user.ID == "" {
		t.Fatalf("el usuario quedó %+v", user)
	}
	found, ok, err := store.User(t.Context(), "berti@ejemplo.com")
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	if !ok || found.ID != user.ID {
		t.Fatalf("el segundo login creó otro usuario: %+v", found)
	}
	link, err := store.CreateLogin(t.Context(), "berti@ejemplo.com", LoginTTL)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	again, err := store.ConsumeLogin(t.Context(), link)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if again.ID != user.ID {
		t.Fatalf("el id cambió: %s contra %s", again.ID, user.ID)
	}
}

func TestALoginLinkWorksOnce(t *testing.T) {
	store := testStore(t)
	link, err := store.CreateLogin(t.Context(), "berti@ejemplo.com", LoginTTL)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	user, err := store.ConsumeLogin(t.Context(), link)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if user.Email != "berti@ejemplo.com" {
		t.Fatalf("dio %q", user.Email)
	}
	if _, err := store.ConsumeLogin(t.Context(), link); err == nil {
		t.Fatal("el link sirvió dos veces")
	}
}

func TestAnExpiredLoginLinkIsRefused(t *testing.T) {
	store := testStore(t)
	link, err := store.CreateLogin(t.Context(), "berti@ejemplo.com", -1)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, err := store.ConsumeLogin(t.Context(), link); err == nil {
		t.Fatal("el link vencido entró")
	}
}

func TestAskedRecentlySaysWhoIsAskingTwice(t *testing.T) {
	store := testStore(t)
	if _, err := store.CreateLogin(t.Context(), "berti@ejemplo.com", LoginTTL); err != nil {
		t.Fatalf("login: %v", err)
	}
	asked, err := store.AskedRecently(t.Context(), "berti@ejemplo.com", 900)
	if err != nil {
		t.Fatalf("asked: %v", err)
	}
	if !asked {
		t.Fatal("no se acordó de quien acaba de pedir")
	}
	other, err := store.AskedRecently(t.Context(), "otro@ejemplo.com", 900)
	if err != nil {
		t.Fatalf("asked: %v", err)
	}
	if other {
		t.Fatal("se acordó de quien nunca pidió")
	}
}

func TestASessionLivesAndDies(t *testing.T) {
	store := testStore(t)
	user := signIn(t, store)
	cookie, err := store.CreateSession(t.Context(), user.ID, SessionTTL)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	found, ok, err := store.Session(t.Context(), cookie)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if !ok || found.ID != user.ID {
		t.Fatalf("la sesión dio %+v %v", found, ok)
	}
	if err := store.DropSession(t.Context(), cookie); err != nil {
		t.Fatalf("drop: %v", err)
	}
	_, ok, err = store.Session(t.Context(), cookie)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if ok {
		t.Fatal("la sesión sobrevivió al logout")
	}
}

func TestAnExpiredSessionIsNoSession(t *testing.T) {
	store := testStore(t)
	user := signIn(t, store)
	cookie, err := store.CreateSession(t.Context(), user.ID, -1)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	_, ok, err := store.Session(t.Context(), cookie)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if ok {
		t.Fatal("la sesión vencida entró")
	}
}

func TestSweepingClearsWhatExpired(t *testing.T) {
	store := testStore(t)
	user := signIn(t, store)
	dead, err := store.CreateLogin(t.Context(), "otro@ejemplo.com", -1)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	old, err := store.CreateSession(t.Context(), user.ID, -1)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	live, err := store.CreateSession(t.Context(), user.ID, SessionTTL)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if err := store.Sweep(t.Context()); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if _, err := store.ConsumeLogin(t.Context(), dead); err == nil {
		t.Fatal("el barrido dejó un link vencido")
	}
	if _, ok, _ := store.Session(t.Context(), old); ok {
		t.Fatal("el barrido dejó una sesión vencida")
	}
	if _, ok, err := store.Session(t.Context(), live); err != nil || !ok {
		t.Fatalf("el barrido se llevó una sesión viva: %v %v", ok, err)
	}
}

func TestNeitherTheLoginNorTheSessionLandsInClear(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	link, err := store.CreateLogin(t.Context(), "berti@ejemplo.com", LoginTTL)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	user, err := store.ConsumeLogin(t.Context(), link)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	session, err := store.CreateSession(t.Context(), user.ID, SessionTTL)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	var stored string
	if err := db.QueryRowContext(t.Context(),
		"SELECT string_agg(hash, '') FROM (SELECT hash FROM auth.logins UNION ALL SELECT hash FROM auth.sessions) h").
		Scan(&stored); err != nil {
		t.Fatalf("no pude leer los hashes: %v", err)
	}
	if strings.Contains(stored, link) || strings.Contains(stored, session) {
		t.Fatal("un token quedó en claro")
	}
}
