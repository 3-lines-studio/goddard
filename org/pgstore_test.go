package org

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
	if _, err := db.ExecContext(t.Context(), "DROP SCHEMA IF EXISTS goddard, auth, chat, heimdall, axe, skill, memo, schedule, org CASCADE; DROP TABLE IF EXISTS public.schema_migrations"); err != nil {
		t.Fatalf("no pude limpiar: %v", err)
	}
	if _, err := migrations.Apply(t.Context(), db); err != nil {
		t.Fatalf("no pude migrar: %v", err)
	}
	return db
}

func testStore(t *testing.T) *PgStore {
	t.Helper()
	return NewPgStore(testDB(t))
}

// somebody is a user of auth, which is what an organization brings in by mail.
func somebody(t *testing.T, store *PgStore, email string) string {
	t.Helper()
	var id string
	err := store.db.QueryRowContext(t.Context(),
		"INSERT INTO auth.users (email, name) VALUES ($1, $2) RETURNING id",
		email, email[:len(email)-len("@ejemplo.com")]).Scan(&id)
	if err != nil {
		t.Fatalf("no pude crear a %s: %v", email, err)
	}
	return id
}

func anOrg(t *testing.T, store *PgStore, name string, ownerID string) Org {
	t.Helper()
	created, err := store.Create(t.Context(), name, ownerID)
	if err != nil {
		t.Fatalf("no pude crear %s: %v", name, err)
	}
	return created
}

func roleIn(t *testing.T, store *PgStore, orgID, userID string) string {
	t.Helper()
	role, ok, err := store.Role(t.Context(), orgID, userID)
	if err != nil {
		t.Fatalf("role: %v", err)
	}
	if !ok {
		t.Fatalf("%s no está en %s", userID, orgID)
	}
	return role
}

func TestAnOrgIsBornWithWhoeverAskedAsItsOwner(t *testing.T) {
	store := testStore(t)
	berti := somebody(t, store, "berti@ejemplo.com")
	created := anOrg(t, store, "La casa", berti)
	if created.Slug != "la-casa" || created.Name != "La casa" || created.ID == "" {
		t.Fatalf("la organización quedó %+v", created)
	}
	found, ok, err := store.Org(t.Context(), created.ID)
	if err != nil || !ok || found.Slug != "la-casa" {
		t.Fatalf("por id quedó %+v (%v, %v)", found, ok, err)
	}
	if role := roleIn(t, store, created.ID, berti); role != RoleOwner {
		t.Fatalf("el creador quedó %q", role)
	}
	orgs, err := store.Orgs(t.Context(), berti)
	if err != nil || len(orgs) != 1 || orgs[0].ID != created.ID {
		t.Fatalf("sus organizaciones quedaron %+v (%v)", orgs, err)
	}
}

func TestTheSameNameTwiceIsTheSameSlugAndIsRefused(t *testing.T) {
	store := testStore(t)
	berti := somebody(t, store, "berti@ejemplo.com")
	ana := somebody(t, store, "ana@ejemplo.com")
	anOrg(t, store, "La casa", berti)
	if _, err := store.Create(t.Context(), "La casa", ana); !errors.Is(err, ErrTaken) {
		t.Fatalf("la segunda contestó %v", err)
	}
	if _, err := store.Create(t.Context(), "   ", ana); err == nil {
		t.Fatal("una organización sin nombre se creó")
	}
}

func TestAnOrgIsOnlyOfThePeopleInIt(t *testing.T) {
	store := testStore(t)
	berti := somebody(t, store, "berti@ejemplo.com")
	ana := somebody(t, store, "ana@ejemplo.com")
	created := anOrg(t, store, "La casa", berti)
	orgs, err := store.Orgs(t.Context(), ana)
	if err != nil {
		t.Fatalf("orgs: %v", err)
	}
	if len(orgs) != 0 {
		t.Fatalf("ana ve %+v sin estar", orgs)
	}
	if err := store.Add(t.Context(), created.ID, "ana@ejemplo.com", RoleMember, berti); err != nil {
		t.Fatalf("add: %v", err)
	}
	orgs, err = store.Orgs(t.Context(), ana)
	if err != nil || len(orgs) != 1 || orgs[0].ID != created.ID {
		t.Fatalf("ahora ve %+v (%v)", orgs, err)
	}
}

func TestAMemberComesInByMailAndEveryoneIsListed(t *testing.T) {
	store := testStore(t)
	berti := somebody(t, store, "berti@ejemplo.com")
	ana := somebody(t, store, "ana@ejemplo.com")
	created := anOrg(t, store, "La casa", berti)
	if err := store.Add(t.Context(), created.ID, "  ANA@Ejemplo.com ", RoleMember, berti); err != nil {
		t.Fatalf("add: %v", err)
	}
	members, err := store.Members(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("quedaron %+v", members)
	}
	if members[0].Role != RoleOwner || members[0].UserID != berti || members[0].Email != "berti@ejemplo.com" || members[0].Name != "berti" {
		t.Fatalf("el dueño quedó %+v", members[0])
	}
	if members[1].Role != RoleMember || members[1].UserID != ana || members[1].Email != "ana@ejemplo.com" {
		t.Fatalf("el miembro quedó %+v", members[1])
	}
}

func TestAMailThatIsNotAnybodysYetIsRefused(t *testing.T) {
	store := testStore(t)
	berti := somebody(t, store, "berti@ejemplo.com")
	created := anOrg(t, store, "La casa", berti)
	if err := store.Add(t.Context(), created.ID, "nadie@ejemplo.com", RoleMember, berti); !errors.Is(err, ErrNoUser) {
		t.Fatalf("contestó %v", err)
	}
	if err := store.Add(t.Context(), created.ID, "ana@ejemplo.com", "dueño", berti); !errors.Is(err, ErrRole) {
		t.Fatalf("un rol que no existe contestó %v", err)
	}
}

func TestAMemberBringsNobodyInAndAnAdminBringsNoOwners(t *testing.T) {
	store := testStore(t)
	berti := somebody(t, store, "berti@ejemplo.com")
	ana := somebody(t, store, "ana@ejemplo.com")
	somebody(t, store, "beto@ejemplo.com")
	created := anOrg(t, store, "La casa", berti)
	if err := store.Add(t.Context(), created.ID, "ana@ejemplo.com", RoleMember, berti); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := store.Add(t.Context(), created.ID, "beto@ejemplo.com", RoleMember, ana); !errors.Is(err, ErrForbidden) {
		t.Fatalf("un miembro agregó y contestó %v", err)
	}
	if err := store.SetRole(t.Context(), created.ID, ana, RoleAdmin, berti); err != nil {
		t.Fatalf("setRole: %v", err)
	}
	if err := store.Add(t.Context(), created.ID, "beto@ejemplo.com", RoleOwner, ana); !errors.Is(err, ErrForbidden) {
		t.Fatalf("un admin nombró dueño y contestó %v", err)
	}
	if err := store.Add(t.Context(), created.ID, "beto@ejemplo.com", RoleMember, ana); err != nil {
		t.Fatalf("un admin no pudo agregar un miembro: %v", err)
	}
}

func TestAnAdminTakesOutMembersAndNobodyElse(t *testing.T) {
	store := testStore(t)
	berti := somebody(t, store, "berti@ejemplo.com")
	ana := somebody(t, store, "ana@ejemplo.com")
	beto := somebody(t, store, "beto@ejemplo.com")
	created := anOrg(t, store, "La casa", berti)
	if err := store.Add(t.Context(), created.ID, "ana@ejemplo.com", RoleAdmin, berti); err != nil {
		t.Fatalf("add ana: %v", err)
	}
	if err := store.Add(t.Context(), created.ID, "beto@ejemplo.com", RoleMember, berti); err != nil {
		t.Fatalf("add beto: %v", err)
	}
	if err := store.Remove(t.Context(), created.ID, beto, ana); err != nil {
		t.Fatalf("un admin no pudo sacar un miembro: %v", err)
	}
	if err := store.Remove(t.Context(), created.ID, berti, ana); !errors.Is(err, ErrForbidden) {
		t.Fatalf("un admin sacó al dueño y contestó %v", err)
	}
	if err := store.Remove(t.Context(), created.ID, ana, ana); !errors.Is(err, ErrForbidden) {
		t.Fatalf("un admin se sacó a sí mismo y contestó %v", err)
	}
	if err := store.Remove(t.Context(), created.ID, beto, berti); !errors.Is(err, ErrNoMember) {
		t.Fatalf("sacar a quien ya no está contestó %v", err)
	}
}

func TestTheLastOwnerStays(t *testing.T) {
	store := testStore(t)
	berti := somebody(t, store, "berti@ejemplo.com")
	ana := somebody(t, store, "ana@ejemplo.com")
	created := anOrg(t, store, "La casa", berti)
	if err := store.Add(t.Context(), created.ID, "ana@ejemplo.com", RoleOwner, berti); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := store.Remove(t.Context(), created.ID, ana, ana); err != nil {
		t.Fatalf("con dos dueños uno no pudo irse: %v", err)
	}
	if err := store.Remove(t.Context(), created.ID, berti, berti); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("el último dueño se fue y contestó %v", err)
	}
	if err := store.SetRole(t.Context(), created.ID, berti, RoleMember, berti); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("el último dueño se bajó de rol y contestó %v", err)
	}
}

func TestOnlyAnOwnerChangesRoles(t *testing.T) {
	store := testStore(t)
	berti := somebody(t, store, "berti@ejemplo.com")
	ana := somebody(t, store, "ana@ejemplo.com")
	beto := somebody(t, store, "beto@ejemplo.com")
	created := anOrg(t, store, "La casa", berti)
	if err := store.Add(t.Context(), created.ID, "ana@ejemplo.com", RoleAdmin, berti); err != nil {
		t.Fatalf("add ana: %v", err)
	}
	if err := store.Add(t.Context(), created.ID, "beto@ejemplo.com", RoleMember, berti); err != nil {
		t.Fatalf("add beto: %v", err)
	}
	if err := store.SetRole(t.Context(), created.ID, beto, RoleAdmin, ana); !errors.Is(err, ErrForbidden) {
		t.Fatalf("un admin cambió un rol y contestó %v", err)
	}
	if err := store.SetRole(t.Context(), created.ID, beto, RoleAdmin, berti); err != nil {
		t.Fatalf("el dueño no pudo cambiar el rol: %v", err)
	}
	if role := roleIn(t, store, created.ID, beto); role != RoleAdmin {
		t.Fatalf("beto quedó %q", role)
	}
	if err := store.SetRole(t.Context(), created.ID, "no-existe", RoleAdmin, berti); !errors.Is(err, ErrNoMember) {
		t.Fatalf("cambiarle el rol a quien no está contestó %v", err)
	}
}

func TestSomebodyComesBackAndKeepsNeitherTheSeatNorTheRole(t *testing.T) {
	store := testStore(t)
	berti := somebody(t, store, "berti@ejemplo.com")
	ana := somebody(t, store, "ana@ejemplo.com")
	created := anOrg(t, store, "La casa", berti)
	if err := store.Add(t.Context(), created.ID, "ana@ejemplo.com", RoleAdmin, berti); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := store.Remove(t.Context(), created.ID, ana, berti); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, ok, err := store.Role(t.Context(), created.ID, ana); err != nil || ok {
		t.Fatalf("ana siguió adentro (%v, %v)", ok, err)
	}
	if err := store.Add(t.Context(), created.ID, "ana@ejemplo.com", RoleMember, berti); err != nil {
		t.Fatalf("add otra vez: %v", err)
	}
	if role := roleIn(t, store, created.ID, ana); role != RoleMember {
		t.Fatalf("al volver quedó %q", role)
	}
	members, err := store.Members(t.Context(), created.ID)
	if err != nil || len(members) != 2 {
		t.Fatalf("quedaron %+v (%v)", members, err)
	}
}

func TestOnlyAnOwnerTakesTheOrgOutAndItLeaves(t *testing.T) {
	store := testStore(t)
	berti := somebody(t, store, "berti@ejemplo.com")
	ana := somebody(t, store, "ana@ejemplo.com")
	created := anOrg(t, store, "La casa", berti)
	if err := store.Add(t.Context(), created.ID, "ana@ejemplo.com", RoleAdmin, berti); err != nil {
		t.Fatalf("no pude meter a ana: %v", err)
	}

	if err := store.Delete(t.Context(), created.ID, ana); !errors.Is(err, ErrForbidden) {
		t.Fatalf("un admin la borró: %v", err)
	}
	if err := store.Delete(t.Context(), created.ID, berti); err != nil {
		t.Fatalf("el dueño no pudo borrarla: %v", err)
	}
	if _, ok, err := store.Org(t.Context(), created.ID); err != nil || ok {
		t.Fatalf("la organización siguió ahí (%v, %v)", ok, err)
	}
	if orgs, err := store.Orgs(t.Context(), berti); err != nil || len(orgs) != 0 {
		t.Fatalf("le quedaron %+v (%v)", orgs, err)
	}
	if _, ok, err := store.Role(t.Context(), created.ID, ana); err != nil || ok {
		t.Fatalf("quedó la membresía de ana (%v, %v)", ok, err)
	}
	if err := store.Delete(t.Context(), created.ID, berti); !errors.Is(err, ErrNoOrg) {
		t.Fatalf("borrarla dos veces contestó %v", err)
	}
}

func TestNobodyOfAnOrgThatIsNotThereDoesAnything(t *testing.T) {
	store := testStore(t)
	berti := somebody(t, store, "berti@ejemplo.com")
	if err := store.Add(t.Context(), "no-existe", "berti@ejemplo.com", RoleMember, berti); !errors.Is(err, ErrNoOrg) {
		t.Fatalf("agregar a una que no existe contestó %v", err)
	}
	if err := store.SetRole(t.Context(), "no-existe", berti, RoleAdmin, berti); !errors.Is(err, ErrNoOrg) {
		t.Fatalf("cambiar un rol contestó %v", err)
	}
	if _, ok, err := store.Org(t.Context(), "no-existe"); err != nil || ok {
		t.Fatalf("buscar una que no existe contestó %v (%v)", ok, err)
	}
}
