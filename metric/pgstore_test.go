package metric

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

// testDB deja la base de los schemas de goddard limpia y con las migraciones
// aplicadas. El candado es el mismo que usan las otras partes para que los
// paquetes que corren en paralelo no se pisen los schemas compartidos.
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
	if _, err := db.ExecContext(t.Context(), "DROP SCHEMA IF EXISTS goddard, auth, chat, heimdall, axe, skill, memo, schedule, org, workspace, metric CASCADE; DROP TABLE IF EXISTS public.schema_migrations"); err != nil {
		t.Fatalf("no pude limpiar: %v", err)
	}
	if _, err := migrations.Apply(t.Context(), db); err != nil {
		t.Fatalf("no pude migrar: %v", err)
	}
	return db
}

func TestARecordedTurnIsThere(t *testing.T) {
	store := NewPgStore(testDB(t))
	turn := Turn{
		OwnerKind:      "user",
		OwnerID:        "01M3",
		ProjectID:      "01PROYECTO",
		ConversationID: "01HILO",
		UserID:         "01M3",
		Source:         "web",
		Model:          "deepseek-flash",
		Input:          1200,
		Output:         340,
		CachedInput:    800,
		Ms:             4200,
		Outcome:        OutcomeOK,
	}
	if err := store.Record(t.Context(), turn); err != nil {
		t.Fatalf("record: %v", err)
	}
	var got Turn
	var id string
	err := store.db.QueryRowContext(t.Context(),
		`SELECT id, owner_kind, owner_id, project_id, conversation_id, user_id, source,
		        model, input, output, cached_input, ms, outcome
		   FROM metric.turns`).
		Scan(&id, &got.OwnerKind, &got.OwnerID, &got.ProjectID, &got.ConversationID,
			&got.UserID, &got.Source, &got.Model, &got.Input, &got.Output,
			&got.CachedInput, &got.Ms, &got.Outcome)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if got != turn {
		t.Fatalf("quedó %+v", got)
	}
	if len(id) != 26 {
		t.Fatalf("el id quedó %q", id)
	}
}

func TestATurnWithoutAnOwnerIsNotRecorded(t *testing.T) {
	store := NewPgStore(testDB(t))
	turn := Turn{OwnerKind: "user", ProjectID: "01P", ConversationID: "01C", Source: "web", Outcome: OutcomeOK}
	if err := store.Record(t.Context(), turn); !errors.Is(err, ErrOwner) {
		t.Fatalf("dio %v", err)
	}
	var count int
	if err := store.db.QueryRowContext(t.Context(), `SELECT count(*) FROM metric.turns`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("quedaron %d filas", count)
	}
}
