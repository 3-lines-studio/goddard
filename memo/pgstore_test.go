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
	if _, err := db.ExecContext(t.Context(), "DROP SCHEMA IF EXISTS memo CASCADE; DROP TABLE IF EXISTS public.schema_migrations"); err != nil {
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

// seed escribe un hecho con la fecha que tenía en jimmy: Add siempre lo fecha
// hoy, y la paridad necesita los días del dump.
func seed(t *testing.T, db *sql.DB, fact Fact) {
	t.Helper()
	_, err := db.ExecContext(t.Context(),
		`INSERT INTO memo.facts (project, key, kind, body, fact_date) VALUES ($1, $2, $3, $4, $5)`,
		fact.Project, fact.Key, fact.Kind, fact.Body, fact.Date)
	if err != nil {
		t.Fatalf("no pude sembrar %q: %v", fact.Key, err)
	}
}

func revisionsOf(t *testing.T, db *sql.DB, key string) []Revision {
	t.Helper()
	rows, err := db.QueryContext(t.Context(),
		`SELECT project, key, rev, kind, body, last_seen FROM memo.revisions WHERE key = $1 ORDER BY rev`, key)
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

	if outcome, err := store.Add(ctx, "jimmy/telemetria", "medicion", "un número"); err != nil || outcome != Created {
		t.Fatalf("el primero quedó %q, %v", outcome, err)
	}
	if outcome, err := store.Add(ctx, "jimmy/telemetria", "medicion", "otro número"); err != nil || outcome != Updated {
		t.Fatalf("el cambiado quedó %q, %v", outcome, err)
	}
	if outcome, err := store.Add(ctx, "jimmy/telemetria", "medicion", "otro número"); err != nil || outcome != Unchanged {
		t.Fatalf("el repetido quedó %q, %v", outcome, err)
	}
	if got := len(revisionsOf(t, db, "jimmy/telemetria")); got != 2 {
		t.Fatalf("guardó %d revisiones donde escribió dos veces", got)
	}

	_, err := db.ExecContext(ctx,
		`UPDATE memo.facts SET fact_date = fact_date - 1 WHERE key = $1`, "jimmy/telemetria")
	if err != nil {
		t.Fatalf("no pude envejecer el hecho: %v", err)
	}
	if outcome, err := store.Add(ctx, "jimmy/telemetria", "medicion", "otro número"); err != nil || outcome != Reasserted {
		t.Fatalf("el reafirmado quedó %q, %v", outcome, err)
	}
}

func TestLasRevisionesGuardanCadaVersion(t *testing.T) {
	store, db := testStore(t)
	ctx := t.Context()
	for _, body := range []string{"uno", "dos", "tres"} {
		if _, err := store.Add(ctx, "usuario", "identidad", body); err != nil {
			t.Fatal(err)
		}
	}
	revisions := revisionsOf(t, db, "usuario")
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
		seed(t, db, fact)
	}
	dump := readTestdata(t, "paridad-rust.txt")
	want := adaptarCola(t, dump["render_tres"])
	got, err := store.Render(t.Context(), "jimmy")
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
		seed(t, db, fact)
	}
	dump := readTestdata(t, "paridad-rust.txt")
	want := adaptarLista(t, dump["list"])
	got, err := store.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("quedó %q\nel Rust dio %q", got, want)
	}
}

func TestShowCaeAlNivelDos(t *testing.T) {
	store, db := testStore(t)
	seed(t, db, entorno(t))
	if _, err := db.ExecContext(t.Context(),
		`INSERT INTO memo.revisions (project, key, rev, kind, body, last_seen) VALUES ($1, $2, 1, $3, $4, $5)`,
		"", "entorno", "plataforma", "linux", day(t, "2026-09-27")); err != nil {
		t.Fatalf("no pude sembrar la revisión: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `DELETE FROM memo.facts WHERE key = 'entorno'`); err != nil {
		t.Fatalf("no pude borrar el hecho: %v", err)
	}
	dump := readTestdata(t, "paridad-rust.txt")
	got, ok, err := store.Show(t.Context(), "entorno")
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
	_, err := run(t.Context(), store, "show", "no-existe", "", "")
	if err == nil {
		t.Fatal("esperaba un error")
	}
	if err.Error() != want {
		t.Fatalf("quedó %q\nel Rust dio %q", err.Error(), want)
	}
}

func TestShowDeUnHechoVivo(t *testing.T) {
	store, db := testStore(t)
	seed(t, db, nueva(t))
	dump := readTestdata(t, "paridad-rust.txt")
	got, ok, err := store.Show(t.Context(), "jimmy/nueva")
	if err != nil || !ok {
		t.Fatalf("el hecho quedó %v, %v", ok, err)
	}
	if got != dump["show_vivo"] {
		t.Fatalf("quedó %q\nel Rust dio %q", got, dump["show_vivo"])
	}
}
