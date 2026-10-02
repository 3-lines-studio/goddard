package chat

import (
	"context"
	"database/sql"
	"errors"
	"os"
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
	if _, err := db.ExecContext(t.Context(), "DROP SCHEMA IF EXISTS goddard, auth, chat, heimdall, axe, skill, memo, schedule, org, workspace CASCADE; DROP TABLE IF EXISTS public.schema_migrations"); err != nil {
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

func project(t *testing.T, store *Store, name string) Project {
	t.Helper()
	found, err := store.CreateProject(t.Context(), name, Owner{Kind: OwnerUser, ID: "berti"}, "berti")
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	return found
}

func TestAProjectKeepsItsNameAndTakesASlug(t *testing.T) {
	store := testStore(t)
	created := project(t, store, "La web de Jimmy")
	if created.Slug != "la-web-de-jimmy" || created.Name != "La web de Jimmy" || created.ID == "" {
		t.Fatalf("quedó %+v", created)
	}
	found, ok, err := store.Project(t.Context(), created.ID)
	if err != nil || !ok {
		t.Fatalf("no lo encontré: %v %v", ok, err)
	}
	if found != created {
		t.Fatalf("volvió %+v", found)
	}
}

func TestTheSlugOfAProjectIsGlobalBecauseItNamesADirectory(t *testing.T) {
	store := testStore(t)
	berti := Owner{Kind: OwnerUser, ID: "berti"}
	ana := Owner{Kind: OwnerUser, ID: "ana"}
	created, err := store.CreateProject(t.Context(), "goddard", berti, "berti")
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if created.Owner.Kind != OwnerUser || created.Owner.ID != "berti" {
		t.Fatalf("el dueño quedó %+v", created.Owner)
	}
	if _, err := store.CreateProject(t.Context(), "goddard", ana, "ana"); !errors.Is(err, ErrTaken) {
		t.Fatalf("el slug goddard quedó libre para otro dueño: %v", err)
	}
	created, err = store.CreateProject(t.Context(), "picsel", Owner{Kind: OwnerOrg, ID: "casa"}, "berti")
	if err != nil {
		t.Fatalf("la org no pudo: %v", err)
	}
	if created.Owner.Kind != OwnerOrg || created.Owner.ID != "casa" {
		t.Fatalf("el dueño quedó %+v", created.Owner)
	}

}
func TestTheSameSlugTwiceIsRefused(t *testing.T) {
	store := testStore(t)
	project(t, store, "Goddard")
	if _, err := store.CreateProject(t.Context(), "goddard", Owner{Kind: OwnerUser, ID: "berti"}, "berti"); !errors.Is(err, ErrTaken) {
		t.Fatalf("dio %v", err)
	}
	if _, err := store.CreateProject(t.Context(), "...", Owner{Kind: OwnerUser, ID: "berti"}, "berti"); err == nil {
		t.Fatal("aceptó un proyecto sin nombre")
	}
}

func TestProjectsComeBackByName(t *testing.T) {
	store := testStore(t)
	project(t, store, "picsel")
	project(t, store, "axe")
	project(t, store, "bifrost")
	names := []string{}
	projects, err := store.Projects(t.Context(), Owner{Kind: OwnerUser, ID: "berti"}, nil)
	if err != nil {
		t.Fatalf("projects: %v", err)
	}
	for _, found := range projects {
		names = append(names, found.Name)
	}
	if len(names) != 3 || names[0] != "axe" || names[1] != "bifrost" || names[2] != "picsel" {
		t.Fatalf("volvieron %v", names)
	}
}

func TestRenamingAProjectKeepsItsSlug(t *testing.T) {
	store := testStore(t)
	created := project(t, store, "ken")
	if err := store.RenameProject(t.Context(), created.ID, "Jimmy"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	found, ok, err := store.Project(t.Context(), created.ID)
	if err != nil || !ok {
		t.Fatalf("no lo encontré: %v %v", ok, err)
	}
	if found.Name != "Jimmy" || found.Slug != "ken" {
		t.Fatalf("quedó %+v", found)
	}
}

func TestADeletedProjectLeavesTheList(t *testing.T) {
	store := testStore(t)
	created := project(t, store, "ken")
	if err := store.DeleteProject(t.Context(), created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	projects, err := store.Projects(t.Context(), Owner{Kind: OwnerUser, ID: "berti"}, nil)
	if err != nil {
		t.Fatalf("projects: %v", err)
	}
	if len(projects) != 0 {
		t.Fatalf("quedaron %+v", projects)
	}
	if _, ok, err := store.Project(t.Context(), created.ID); err != nil || ok {
		t.Fatalf("el borrado volvió: %v %v", ok, err)
	}
}

func TestDeletingTheProjectsOfAnOwner(t *testing.T) {
	store := testStore(t)
	org := Owner{Kind: OwnerOrg, ID: "acme"}
	mine, err := store.CreateProject(t.Context(), "de-la-org", org, "berti")
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	other, err := store.CreateProject(t.Context(), "de-otra-org", Owner{Kind: OwnerOrg, ID: "otra"}, "berti")
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	mineToo := project(t, store, "mio")

	if err := store.DeleteProjects(t.Context(), org); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok, err := store.Project(t.Context(), mine.ID); err != nil || ok {
		t.Fatalf("el de la org siguió ahí (%v, %v)", ok, err)
	}
	if _, ok, err := store.Project(t.Context(), other.ID); err != nil || !ok {
		t.Fatalf("se llevó el de otra org (%v, %v)", ok, err)
	}
	if _, ok, err := store.Project(t.Context(), mineToo.ID); err != nil || !ok {
		t.Fatalf("se llevó el propio (%v, %v)", ok, err)
	}
	if err := store.DeleteProjects(t.Context(), Owner{Kind: OwnerOrg, ID: "vacia"}); err != nil {
		t.Fatalf("borrar los de una org sin proyectos contestó %v", err)
	}
}

func TestCreatingAnArchivedProjectBringsItBack(t *testing.T) {
	store := testStore(t)
	created := project(t, store, "ken")
	thread, err := store.CreateConversation(t.Context(), created.ID, "el esquema", SourceWeb, "berti")
	if err != nil {
		t.Fatalf("conversation: %v", err)
	}
	if err := store.DeleteProject(t.Context(), created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	back, err := store.CreateProject(t.Context(), "Ken", Owner{Kind: OwnerUser, ID: "berti"}, "berti")
	if err != nil {
		t.Fatalf("no volvió: %v", err)
	}
	if back.ID != created.ID || back.Slug != "ken" || back.Name != "ken" {
		t.Fatalf("volvió otro: %+v", back)
	}
	threads, err := store.Conversations(t.Context(), back.ID)
	if err != nil || len(threads) != 1 || threads[0].ID != thread.ID {
		t.Fatalf("volvió con %d conversaciones (%v)", len(threads), err)
	}
	if found, ok, err := store.Project(t.Context(), created.ID); err != nil || !ok || found != created {
		t.Fatalf("el proyecto quedó %+v (%v, %v)", found, ok, err)
	}
}

func TestAnArchivedProjectIsStillTakenForAnotherOwner(t *testing.T) {
	store := testStore(t)
	created := project(t, store, "ken")
	if err := store.DeleteProject(t.Context(), created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.CreateProject(t.Context(), "ken", Owner{Kind: OwnerUser, ID: "ana"}, "ana"); !errors.Is(err, ErrTaken) {
		t.Fatalf("otro dueño se llevó el slug archivado: %v", err)
	}
	if found, ok, err := store.Project(t.Context(), created.ID); err != nil || ok || found != (Project{}) {
		t.Fatalf("el archivado se despertó solo: %+v (%v, %v)", found, ok, err)
	}
}

func TestConversationsLiveInsideAProject(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	created, err := store.CreateProject(t.Context(), "goddard", Owner{Kind: OwnerUser, ID: "berti"}, "berti")
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	first, err := store.CreateConversation(t.Context(), created.ID, "", "", "berti")
	if err != nil {
		t.Fatalf("conversation: %v", err)
	}
	if first.Title != NewTitle || first.Source != SourceWeb {
		t.Fatalf("quedó %+v", first)
	}
	second, err := store.CreateConversation(t.Context(), created.ID, "el esquema", SourceTelegram, "berti")
	if err != nil {
		t.Fatalf("conversation: %v", err)
	}
	list, err := store.Conversations(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("conversations: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("volvieron %d", len(list))
	}
	if _, err := db.ExecContext(t.Context(),
		"UPDATE chat.conversations SET updated_at = CASE id WHEN $1 THEN 100 WHEN $2 THEN 200 END",
		first.ID, second.ID); err != nil {
		t.Fatalf("no pude separarlas en el tiempo: %v", err)
	}
	list, err = store.Conversations(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("conversations: %v", err)
	}
	if list[0].ID != second.ID || list[1].ID != first.ID {
		t.Fatalf("el orden quedó %+v", list)
	}
	found, ok, err := store.Conversation(t.Context(), first.ID)
	if err != nil || !ok {
		t.Fatalf("no la encontré: %v %v", ok, err)
	}
	if found.ProjectID != created.ID || found.Title != NewTitle {
		t.Fatalf("volvió %+v", found)
	}
}

func TestARenamedConversationSaysWhatItIsAbout(t *testing.T) {
	store := testStore(t)
	created := project(t, store, "goddard")
	thread, err := store.CreateConversation(t.Context(), created.ID, "", "", "berti")
	if err != nil {
		t.Fatalf("conversation: %v", err)
	}
	if err := store.RenameConversation(t.Context(), thread.ID, "el esquema"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	found, ok, err := store.Conversation(t.Context(), thread.ID)
	if err != nil || !ok {
		t.Fatalf("no la encontré: %v %v", ok, err)
	}
	if found.Title != "el esquema" {
		t.Fatalf("quedó %+v", found)
	}
}

func TestADeletedConversationLeavesTheList(t *testing.T) {
	store := testStore(t)
	created := project(t, store, "goddard")
	thread, err := store.CreateConversation(t.Context(), created.ID, "el esquema", "", "berti")
	if err != nil {
		t.Fatalf("conversation: %v", err)
	}
	if err := store.DeleteConversation(t.Context(), thread.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, err := store.Conversations(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("conversations: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("quedaron %+v", list)
	}
}

func TestAConversationCannotComeFromAnywhere(t *testing.T) {
	store := testStore(t)
	created := project(t, store, "goddard")
	if _, err := store.CreateConversation(t.Context(), created.ID, "x", "carrier-pigeon", "berti"); err == nil {
		t.Fatal("aceptó un origen que no existe")
	}
	if _, err := store.CreateConversation(t.Context(), "no-existe", "x", "", "berti"); err == nil {
		t.Fatal("aceptó un proyecto que no existe")
	}
}
