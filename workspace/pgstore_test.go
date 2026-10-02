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

func TestAnOwnerWithoutARowHasNoWorkspace(t *testing.T) {
	found, ok, err := NewPgStore(testDB(t)).Get(t.Context(), Owner{Kind: OwnerUser, ID: "01M3"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if ok {
		t.Fatalf("devolvió %+v", found)
	}
	if found.Path != "" {
		t.Fatalf("el path quedó %q", found.Path)
	}
}

func TestAWorkspaceKeepsWhatIsWritten(t *testing.T) {
	store := NewPgStore(testDB(t))
	owner := Owner{Kind: OwnerOrg, ID: "01M4"}
	wanted := Workspace{Owner: owner, Path: "/data/volumes/la-casa"}
	if err := store.Set(t.Context(), wanted, "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	found, ok, err := store.Get(t.Context(), owner)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ok || found.Path != wanted.Path {
		t.Fatalf("volvió %+v", found)
	}
	if found.ProjectDir("goddard") != "/data/volumes/la-casa/goddard" {
		t.Fatalf("el directorio del proyecto quedó %q", found.ProjectDir("goddard"))
	}
	wanted.Path = "/data/volumes/otra-casa"
	if err := store.Set(t.Context(), wanted, "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	found, ok, err = store.Get(t.Context(), owner)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ok || found.Path != "/data/volumes/otra-casa" {
		t.Fatalf("el path quedó %q", found.Path)
	}
}

func TestSetRefusesWhatCannotBeWritten(t *testing.T) {
	store := NewPgStore(testDB(t))
	space := Workspace{Owner: Owner{Kind: OwnerUser, ID: "uno"}, Path: "volumes/uno"}
	if err := store.Set(t.Context(), space, "berti"); err == nil {
		t.Fatal("escribió un path relativo")
	}
}

func TestDeletingAWorkspaceLeavesTheDefaultAgain(t *testing.T) {
	store := NewPgStore(testDB(t))
	owner := Owner{Kind: OwnerUser, ID: "01M5"}
	if err := store.Set(t.Context(), Workspace{Owner: owner, Path: "/data/volumes/la-casa"}, "berti"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := store.Delete(t.Context(), owner); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, ok, err := store.Get(t.Context(), owner)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if ok {
		t.Fatal("la fila quedó")
	}
	if err := store.Delete(t.Context(), owner); err != nil {
		t.Fatalf("borrar dos veces: %v", err)
	}
}
