package memo

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/3-lines-studio/goddard/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const testLock = 0x676f6464544553

// testDB deja la base del schema memo limpia y con las migraciones aplicadas.
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

func testStore(t *testing.T) (*PgStore, *sql.DB) {
	t.Helper()
	db := testDB(t)
	return NewPgStore(db), db
}

// testScope is the memory of a person inside a project, with the two owners
// apart on purpose: the general facts are theirs and the project's belong to
// the organization that owns it.
func testScope() Scope {
	return Scope{
		User:    Owner{Kind: KindUser, ID: "u1"},
		Project: Owner{Kind: KindOrg, ID: "o1"},
		Slug:    "jimmy",
	}
}

// seed escribe un hecho con la fecha que tenía en jimmy: Add siempre lo fecha
// hoy, y la paridad necesita los días del dump. El dueño sale de la clave, como
// en Add.
func seed(t *testing.T, db *sql.DB, scope Scope, fact Fact) {
	t.Helper()
	owner, project := scope.whereOf(fact.Key)
	_, err := db.ExecContext(t.Context(),
		`INSERT INTO memo.facts (owner_kind, owner_id, project, key, kind, body, fact_date)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		owner.Kind, owner.ID, project, fact.Key, fact.Kind, fact.Body, fact.Date)
	if err != nil {
		t.Fatalf("no pude sembrar %q: %v", fact.Key, err)
	}
}

func revisionsOf(t *testing.T, db *sql.DB, scope Scope, key string) []Revision {
	t.Helper()
	owner, project := scope.whereOf(key)
	rows, err := db.QueryContext(t.Context(),
		`SELECT project, key, rev, kind, body, last_seen FROM memo.revisions
		 WHERE owner_kind = $1 AND owner_id = $2 AND project = $3 AND key = $4 ORDER BY rev`,
		owner.Kind, owner.ID, project, key)
	if err != nil {
		t.Fatalf("no pude leer las revisiones de %q: %v", key, err)
	}
	defer rows.Close()
	found := []Revision{}
	for rows.Next() {
		var revision Revision
		if err := rows.Scan(&revision.Project, &revision.Key, &revision.Rev, &revision.Kind, &revision.Body, &revision.LastSeen); err != nil {
			t.Fatalf("no pude leer una revisión de %q: %v", key, err)
		}
		found = append(found, revision)
	}
	return found
}

func TestAddDiceQueHizo(t *testing.T) {
	store, db := testStore(t)
	ctx := t.Context()
	scope := testScope()

	if outcome, err := store.Add(ctx, scope, "jimmy/telemetria", "medicion", "un número"); err != nil || outcome != Created {
		t.Fatalf("el primero quedó %q, %v", outcome, err)
	}
	if outcome, err := store.Add(ctx, scope, "jimmy/telemetria", "medicion", "otro número"); err != nil || outcome != Updated {
		t.Fatalf("el cambiado quedó %q, %v", outcome, err)
	}
	if outcome, err := store.Add(ctx, scope, "jimmy/telemetria", "medicion", "otro número"); err != nil || outcome != Unchanged {
		t.Fatalf("el repetido quedó %q, %v", outcome, err)
	}
	if got := len(revisionsOf(t, db, scope, "jimmy/telemetria")); got != 2 {
		t.Fatalf("guardó %d revisiones donde escribió dos veces", got)
	}

	_, err := db.ExecContext(ctx,
		`UPDATE memo.facts SET fact_date = fact_date - 1 WHERE key = $1`, "jimmy/telemetria")
	if err != nil {
		t.Fatalf("no pude envejecer el hecho: %v", err)
	}
	if outcome, err := store.Add(ctx, scope, "jimmy/telemetria", "medicion", "otro número"); err != nil || outcome != Reasserted {
		t.Fatalf("el reafirmado quedó %q, %v", outcome, err)
	}
}

func TestLasRevisionesGuardanCadaVersion(t *testing.T) {
	store, db := testStore(t)
	ctx := t.Context()
	scope := testScope()
	for _, body := range []string{"uno", "dos", "tres"} {
		if _, err := store.Add(ctx, scope, "usuario", "identidad", body); err != nil {
			t.Fatal(err)
		}
	}
	revisions := revisionsOf(t, db, scope, "usuario")
	if len(revisions) != 3 {
		t.Fatalf("quedaron %d revisiones", len(revisions))
	}
	for index, revision := range revisions {
		if revision.Rev != index+1 {
			t.Fatalf("la revisión %d tiene rev %d", index, revision.Rev)
		}
		if revision.Body != []string{"uno", "dos", "tres"}[index] {
			t.Fatalf("la revisión %d quedó %q", index, revision.Body)
		}
	}
}

func TestRenderEsElDeJimmy(t *testing.T) {
	store, db := testStore(t)
	for _, fact := range todo(t) {
		seed(t, db, testScope(), fact)
	}
	dump := readTestdata(t, "paridad-rust.txt")
	want := adaptarCola(t, dump["render_tres"])
	got, err := store.Render(t.Context(), testScope())
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("quedó %q\nel Rust dio %q", got, want)
	}
}

func TestListNombraLosAmbitos(t *testing.T) {
	store, db := testStore(t)
	for _, fact := range todo(t) {
		seed(t, db, testScope(), fact)
	}
	dump := readTestdata(t, "paridad-rust.txt")
	want := adaptarLista(t, dump["list"])
	got, err := store.List(t.Context(), testScope())
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("quedó %q\nel Rust dio %q", got, want)
	}
}

func TestShowCaeAlNivelDos(t *testing.T) {
	store, db := testStore(t)
	scope := testScope()
	seed(t, db, scope, entorno(t))
	if _, err := db.ExecContext(t.Context(),
		`INSERT INTO memo.revisions (owner_kind, owner_id, project, key, rev, kind, body, last_seen)
		 VALUES ($1, $2, $3, $4, 1, $5, $6, $7)`,
		scope.User.Kind, scope.User.ID, "", "entorno", "plataforma", "linux", day(t, "2026-09-27")); err != nil {
		t.Fatalf("no pude sembrar la revisión: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `DELETE FROM memo.facts WHERE key = 'entorno'`); err != nil {
		t.Fatalf("no pude borrar el hecho: %v", err)
	}
	dump := readTestdata(t, "paridad-rust.txt")
	got, ok, err := store.Show(t.Context(), scope, "entorno")
	if err != nil || !ok {
		t.Fatalf("el hecho borrado quedó %v, %v", ok, err)
	}
	if got != dump["show_borrado"] {
		t.Fatalf("quedó %q\nel Rust dio %q", got, dump["show_borrado"])
	}
}

func TestShowNoExiste(t *testing.T) {
	store, _ := testStore(t)
	dump := readTestdata(t, "paridad-rust.txt")
	want := adaptarError(t, dump["show_no_existe"])
	_, err := run(t.Context(), store, testScope(), "show", "no-existe", "", "")
	if err == nil {
		t.Fatal("esperaba un error")
	}
	if err.Error() != want {
		t.Fatalf("quedó %q\nel Rust dio %q", err.Error(), want)
	}
}

func TestShowDeUnHechoVivo(t *testing.T) {
	store, db := testStore(t)
	seed(t, db, testScope(), nueva(t))
	dump := readTestdata(t, "paridad-rust.txt")
	got, ok, err := store.Show(t.Context(), testScope(), "jimmy/nueva")
	if err != nil || !ok {
		t.Fatalf("el hecho quedó %v, %v", ok, err)
	}
	if got != dump["show_vivo"] {
		t.Fatalf("quedó %q\nel Rust dio %q", got, dump["show_vivo"])
	}
}

func TestFactsTraeLoDelProyectoYLoGeneral(t *testing.T) {
	store, db := testStore(t)
	for _, fact := range todo(t) {
		seed(t, db, testScope(), fact)
	}
	facts, err := store.Facts(t.Context(), testScope())
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	otros := map[string]bool{}
	for _, fact := range facts {
		seen[fact.Project] = true
		if fact.Project != "" && fact.Project != "jimmy" {
			otros[fact.Project] = true
		}
	}
	if !seen[""] {
		t.Fatal("no trajo los hechos generales")
	}
	if !seen["jimmy"] {
		t.Fatal("no trajo los del proyecto")
	}
	if len(otros) > 0 {
		t.Fatalf("se colaron hechos de otros proyectos: %v", otros)
	}
}

func TestLaGeneralEsDeCadaUnoYLaDelProyectoEsDelProyecto(t *testing.T) {
	store, _ := testStore(t)
	ctx := t.Context()
	casa := Owner{Kind: KindOrg, ID: "o1"}
	berti := Scope{User: Owner{Kind: KindUser, ID: "u1"}, Project: casa, Slug: "jimmy"}
	ana := Scope{User: Owner{Kind: KindUser, ID: "u2"}, Project: casa, Slug: "jimmy"}

	if _, err := store.Add(ctx, berti, "usuario", "identidad", "es berti"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(ctx, ana, "usuario", "identidad", "es ana"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(ctx, berti, "jimmy/deploy", "estado", "sale de staging"); err != nil {
		t.Fatal(err)
	}

	keys := func(scope Scope) map[string]string {
		t.Helper()
		facts, err := store.Facts(ctx, scope)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]string{}
		for _, fact := range facts {
			out[fact.Key] = fact.Body
		}
		return out
	}
	mine, theirs := keys(berti), keys(ana)
	if len(mine) != 2 || mine["usuario"] != "es berti" || mine["jimmy/deploy"] != "sale de staging" {
		t.Fatalf("berti ve %+v", mine)
	}
	if len(theirs) != 2 || theirs["usuario"] != "es ana" || theirs["jimmy/deploy"] != "sale de staging" {
		t.Fatalf("ana ve %+v", theirs)
	}

	// y el mismo proyecto de la misma organización es el mismo hecho: el
	// segundo que lo escribe lo reemplaza, y los dos lo leen.
	if outcome, err := store.Add(ctx, ana, "jimmy/deploy", "estado", "ahora sale de main"); err != nil || outcome != Updated {
		t.Fatalf("el hecho del proyecto quedó %q, %v", outcome, err)
	}
	if mine, theirs = keys(berti), keys(ana); mine["jimmy/deploy"] != "ahora sale de main" || theirs["jimmy/deploy"] != "ahora sale de main" {
		t.Fatalf("el hecho del proyecto quedó %q y %q", mine["jimmy/deploy"], theirs["jimmy/deploy"])
	}
}

func TestElProyectoDeOtroNoSeVe(t *testing.T) {
	store, _ := testStore(t)
	ctx := t.Context()
	mine := Scope{User: Owner{Kind: KindUser, ID: "u1"}, Project: Owner{Kind: KindUser, ID: "u1"}, Slug: "jimmy"}
	other := Scope{User: Owner{Kind: KindUser, ID: "u2"}, Project: Owner{Kind: KindUser, ID: "u2"}, Slug: "jimmy"}

	if _, err := store.Add(ctx, other, "jimmy/deploy", "estado", "lo de ana"); err != nil {
		t.Fatal(err)
	}
	facts, err := store.Facts(ctx, mine)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 0 {
		t.Fatalf("el proyecto del otro se ve: %+v", facts)
	}
	if found, ok, err := store.Show(ctx, mine, "jimmy/deploy"); err != nil || ok || found != "" {
		t.Fatalf("el hecho del otro se lee: %q, %v, %v", found, ok, err)
	}
}
