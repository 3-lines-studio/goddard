package skill

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/3-lines-studio/goddard/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const testLock = 0x676f6464544553

// testDB deja la base del schema skill limpia y con las migraciones aplicadas.
// El candado es el mismo que usan las otras partes de goddard para que los
// paquetes que corren en paralelo no se pisen el schema compartido.
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

func testStore(t *testing.T) (*PgStore, Viewer) {
	t.Helper()
	return NewPgStore(testDB(t)), Viewer{Orgs: []string{"o1"}, User: "u1", Role: "member"}
}

func put(t *testing.T, store *PgStore, viewer Viewer, skill Skill) {
	t.Helper()
	if err := store.Put(t.Context(), viewer, skill); err != nil {
		t.Fatalf("no pude guardar %q: %v", skill.Name, err)
	}
}

func TestPutAndLoadKeepTheBodyAsItIs(t *testing.T) {
	store, viewer := testStore(t)
	body := "# browse\n\n- uno\n- dos\n\n\n  sangría al final\n"
	put(t, store, viewer, Skill{
		Meta: Meta{Owner: Owner{Kind: Org, ID: "o1"}, Name: "browse", Description: "Navegar"},
		Body: body,
	})
	got, err := store.Load(t.Context(), viewer, "browse")
	if err != nil {
		t.Fatal(err)
	}
	want := "Skill browse\n\n" + body
	if got != want {
		t.Fatalf("quedó %q", got)
	}
}

func TestListIsAlphabeticalAndIndexed(t *testing.T) {
	store, viewer := testStore(t)
	put(t, store, viewer, Skill{Meta: Meta{Owner: Owner{Kind: Org, ID: "o1"}, Name: "previews", Description: "Servir un proyecto"}})
	put(t, store, viewer, Skill{Meta: Meta{Owner: Owner{Kind: Org, ID: "o1"}, Name: "browse", Description: "Navegar"}})
	metas, err := store.List(t.Context(), viewer)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 2 || metas[0].Name != "browse" || metas[1].Name != "previews" {
		t.Fatalf("la lista quedó %+v", metas)
	}
	index, err := store.Index(t.Context(), viewer)
	if err != nil {
		t.Fatal(err)
	}
	if index != "browse — Navegar\npreviews — Servir un proyecto" {
		t.Fatalf("el índice quedó %q", index)
	}
}

func TestIndexIsEmptyWithoutSkills(t *testing.T) {
	store, viewer := testStore(t)
	index, err := store.Index(t.Context(), viewer)
	if err != nil {
		t.Fatal(err)
	}
	if index != EmptyIndex {
		t.Fatalf("quedó %q", index)
	}
}

func TestTheClosestOwnerWins(t *testing.T) {
	store, viewer := testStore(t)
	putSystem(t, store, "browse", "De fábrica", "el del sistema")
	put(t, store, viewer, Skill{Meta: Meta{Owner: Owner{Kind: Org, ID: "o1"}, Name: "browse", Description: "De la org"}, Body: "el de la org"})
	put(t, store, viewer, Skill{Meta: Meta{Owner: Owner{Kind: User, ID: "u1"}, Name: "browse", Description: "Mía"}, Body: "el mío"})
	index, err := store.Index(t.Context(), viewer)
	if err != nil {
		t.Fatal(err)
	}
	if index != "browse — Mía" {
		t.Fatalf("quedó %q", index)
	}
	loaded, err := store.Load(t.Context(), viewer, "browse")
	if err != nil {
		t.Fatal(err)
	}
	if loaded != "Skill browse\n\nel mío" {
		t.Fatalf("cargó %q", loaded)
	}
}

func TestTheSystemShowsForEveryoneButLosesToTheCloser(t *testing.T) {
	store, viewer := testStore(t)
	putSystem(t, store, "browse", "De fábrica", "el del sistema")
	stranger := Viewer{Orgs: []string{"o9"}, User: "u9"}
	metas, err := store.List(t.Context(), stranger)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 || metas[0].Owner.Kind != System {
		t.Fatalf("el ajeno vio %+v", metas)
	}
	put(t, store, viewer, Skill{Meta: Meta{Owner: Owner{Kind: Org, ID: "o1"}, Name: "browse", Description: "De la org"}})
	found, ok, err := store.Get(t.Context(), viewer, "browse")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || found.Owner.Kind != Org {
		t.Fatalf("quedó %+v", found)
	}
}

func putSystem(t *testing.T, store *PgStore, name, description, body string) {
	t.Helper()
	_, err := store.db.ExecContext(t.Context(),
		`INSERT INTO skill.skills (owner_kind, owner_id, role, name, description, body, created_by, created_at, updated_at)
		 VALUES ('system', '', '', $1, $2, $3, '', $4, $4)`,
		name, description, body, nowMs())
	if err != nil {
		t.Fatalf("no pude sembrar %q: %v", name, err)
	}
}

func TestAnotherOrganizationSeesNothing(t *testing.T) {
	store, viewer := testStore(t)
	put(t, store, viewer, Skill{Meta: Meta{Owner: Owner{Kind: Org, ID: "o1"}, Name: "browse", Description: "Navegar"}})
	intruder := Viewer{Orgs: []string{"o2"}, User: "u2"}
	metas, err := store.List(t.Context(), intruder)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 0 {
		t.Fatalf("el ajeno vio %+v", metas)
	}
	if _, ok, _ := store.Get(t.Context(), intruder, "browse"); ok {
		t.Fatal("el ajeno cargó un skill de la organización")
	}
}

func TestSomebodyInTwoOrganizationsSeesBoth(t *testing.T) {
	store, _ := testStore(t)
	planter := Viewer{Orgs: []string{"o1", "o2"}, User: "u1"}
	put(t, store, planter, Skill{Meta: Meta{Owner: Owner{Kind: Org, ID: "o1"}, Name: "browse", Description: "De la primera"}})
	put(t, store, planter, Skill{Meta: Meta{Owner: Owner{Kind: Org, ID: "o2"}, Name: "deploy", Description: "De la segunda"}})
	inTwo := Viewer{Orgs: []string{"o2", "o3"}, User: "u2"}
	metas, err := store.List(t.Context(), inTwo)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 || metas[0].Name != "deploy" {
		t.Fatalf("sólo en la segunda vio %+v", metas)
	}
	inTwo.Orgs = []string{"o1", "o2"}
	metas, err = store.List(t.Context(), inTwo)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 2 {
		t.Fatalf("en las dos vio %+v", metas)
	}
}

func TestARoleLimitsWhoSeesIt(t *testing.T) {
	store, admin := testStore(t)
	admin.Role = "admin"
	put(t, store, admin, Skill{Meta: Meta{Owner: Owner{Kind: Org, ID: "o1"}, Role: "admin", Name: "auditoria", Description: "Sólo admins"}})
	member := Viewer{Orgs: []string{"o1"}, User: "u2", Role: "member"}
	metas, err := store.List(t.Context(), member)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 0 {
		t.Fatalf("el member vio %+v", metas)
	}
	put(t, store, admin, Skill{Meta: Meta{Owner: Owner{Kind: Org, ID: "o1"}, Name: "browse", Description: "Para todos"}})
	metas, err = store.List(t.Context(), member)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 || metas[0].Name != "browse" {
		t.Fatalf("el member vio %+v", metas)
	}
}

func TestNobodyWritesTheSystem(t *testing.T) {
	store, viewer := testStore(t)
	owner := Owner{Kind: System}
	err := store.Put(t.Context(), viewer, Skill{Meta: Meta{Owner: owner, Name: "browse", Description: "De fábrica"}})
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Put quedó %v", err)
	}
	if _, err := store.Delete(t.Context(), viewer, owner, "browse"); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Delete quedó %v", err)
	}
}

func TestNobodyWritesSomebodyElsesStuff(t *testing.T) {
	store, viewer := testStore(t)
	err := store.Put(t.Context(), viewer, Skill{Meta: Meta{Owner: Owner{Kind: Org, ID: "o2"}, Name: "browse", Description: "Ajena"}})
	if err == nil {
		t.Fatal("escribió en otra organización")
	}
	if _, err := store.Delete(t.Context(), viewer, Owner{Kind: User, ID: "u9"}, "browse"); err == nil {
		t.Fatal("borró de otro usuario")
	}
}

func TestPutReplacesAndKeepsWhenItWasCreated(t *testing.T) {
	store, viewer := testStore(t)
	owner := Owner{Kind: User, ID: "u1"}
	put(t, store, viewer, Skill{Meta: Meta{Owner: owner, Name: "mía", Description: "Primera"}, Body: "uno"})
	first, _, err := store.Get(t.Context(), viewer, "mía")
	if err != nil {
		t.Fatal(err)
	}
	if first.CreatedBy != "u1" {
		t.Fatalf("el autor quedó %q", first.CreatedBy)
	}
	time.Sleep(2 * time.Millisecond)
	put(t, store, viewer, Skill{Meta: Meta{Owner: owner, Name: "mía", Description: "Segunda"}, Body: "dos"})
	second, _, err := store.Get(t.Context(), viewer, "mía")
	if err != nil {
		t.Fatal(err)
	}
	if second.Description != "Segunda" || second.Body != "dos" {
		t.Fatalf("no se reemplazó: %+v", second)
	}
	if second.CreatedAt != first.CreatedAt {
		t.Fatalf("created_at cambió: %d contra %d", second.CreatedAt, first.CreatedAt)
	}
	if second.UpdatedAt <= first.UpdatedAt {
		t.Fatalf("updated_at no avanzó: %d contra %d", second.UpdatedAt, first.UpdatedAt)
	}
}

func TestListKeepsOneRowPerName(t *testing.T) {
	store, viewer := testStore(t)
	put(t, store, viewer, Skill{Meta: Meta{Owner: Owner{Kind: Org, ID: "o1"}, Name: "browse", Description: "De la org"}})
	put(t, store, viewer, Skill{Meta: Meta{Owner: Owner{Kind: User, ID: "u1"}, Name: "browse", Description: "Mía"}})
	put(t, store, viewer, Skill{Meta: Meta{Owner: Owner{Kind: User, ID: "u1"}, Name: "otra", Description: "Otra"}})
	metas, err := store.List(t.Context(), viewer)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 2 {
		t.Fatalf("quedaron %d: %+v", len(metas), metas)
	}
}

func TestDeleteSaysWhetherThereWasOne(t *testing.T) {
	store, viewer := testStore(t)
	owner := Owner{Kind: User, ID: "u1"}
	put(t, store, viewer, Skill{Meta: Meta{Owner: owner, Name: "mía", Description: "Mía"}})
	deleted, err := store.Delete(t.Context(), viewer, owner, "mía")
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("dijo que no estaba")
	}
	again, err := store.Delete(t.Context(), viewer, owner, "mía")
	if err != nil {
		t.Fatal(err)
	}
	if again {
		t.Fatal("dijo que estaba dos veces")
	}
}

func TestLoadComplainsAboutWhatIsNotThere(t *testing.T) {
	store, viewer := testStore(t)
	if _, err := store.Load(t.Context(), viewer, "nope"); err == nil {
		t.Fatal("no chilló por la que no existe")
	}
	if _, err := store.Load(t.Context(), viewer, "../secrets"); err == nil {
		t.Fatal("no chilló por el nombre malo")
	}
	if err := store.Put(t.Context(), viewer, Skill{Meta: Meta{Owner: Owner{Kind: User, ID: "u1"}, Name: "a/b"}}); err == nil {
		t.Fatal("guardó un nombre malo")
	}
}

func TestTheSchemaRejectsAnUnknownOwner(t *testing.T) {
	db := testDB(t)
	_, err := db.ExecContext(t.Context(),
		`INSERT INTO skill.skills VALUES ('equipo', 'e1', '', 'x', 'd', 'b', 'u1', 1, 1)`)
	if err == nil {
		t.Fatal("el CHECK dejó pasar un dueño desconocido")
	}
}
