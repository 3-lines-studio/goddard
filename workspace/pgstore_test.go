package workspace

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/3-lines-studio/goddard/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const testLock = 0x676f6464544553

// testDB deja la base del schema workspace limpia y con las migraciones
// aplicadas. El candado es el mismo que usan las otras partes de goddard para
// que los paquetes que corren en paralelo no se pisen el schema compartido.
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
	if _, err := db.ExecContext(t.Context(), "DROP SCHEMA IF EXISTS goddard, auth, chat, heimdall, axe, skill, memo, schedule, org, workspace CASCADE; DROP TABLE IF EXISTS public.schema_migrations"); err != nil {
		t.Fatalf("no pude limpiar: %v", err)
	}
	if _, err := migrations.Apply(t.Context(), db); err != nil {
		t.Fatalf("no pude migrar: %v", err)
	}
	return db
}

func TestAWorkspaceExistsWithoutARow(t *testing.T) {
	store := NewPgStore(testDB(t))
	found, err := store.Get(t.Context(), Owner{Kind: OwnerUser, ID: "01M3"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if found.Path != Volumes+"/01M3" {
		t.Fatalf("el path quedó %q", found.Path)
	}
	if found.Sandbox.Kind != SandboxSSH {
		t.Fatalf("el sandbox quedó %+v", found.Sandbox)
	}
	if found.Reachable() {
		t.Fatal("dijo que el sandbox se alcanza")
	}
}

func TestAWorkspaceKeepsWhatIsWritten(t *testing.T) {
	store := NewPgStore(testDB(t))
	owner := Owner{Kind: OwnerOrg, ID: "01M4"}
	wanted := New(owner)
	wanted.Path = "/data/volumes/la-casa"
	wanted.Sandbox = Sandbox{Kind: SandboxSSH, Addr: "sandbox.local:22", User: "goddard"}
	if err := store.Set(t.Context(), wanted, "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	found, err := store.Get(t.Context(), owner)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if found.Path != wanted.Path || found.Sandbox != wanted.Sandbox {
		t.Fatalf("volvió %+v", found)
	}
	if found.ProjectDir("goddard") != "/data/volumes/la-casa/goddard" {
		t.Fatalf("el directorio del proyecto quedó %q", found.ProjectDir("goddard"))
	}
	wanted.Sandbox.Addr = "otro.local:22"
	if err := store.Set(t.Context(), wanted, "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	found, err = store.Get(t.Context(), owner)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if found.Sandbox.Addr != "otro.local:22" {
		t.Fatalf("el sandbox quedó %+v", found.Sandbox)
	}
}

func TestSetRefusesWhatCannotBeWritten(t *testing.T) {
	store := NewPgStore(testDB(t))
	space := New(Owner{Kind: OwnerUser, ID: "uno"})
	space.Path = "volumes/uno"
	if err := store.Set(t.Context(), space, "berti"); err == nil {
		t.Fatal("escribió un path relativo")
	}
}
