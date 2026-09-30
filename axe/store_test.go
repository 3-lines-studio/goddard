package axe

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"reflect"
	"sync"
	"testing"

	"github.com/3-lines-studio/goddard/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const testLock = 0x676f6464544553

// testDB deja la base del schema axe limpia y con las migraciones aplicadas.
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
	if _, err := db.ExecContext(t.Context(), "DROP SCHEMA IF EXISTS axe CASCADE; DROP TABLE IF EXISTS public.schema_migrations"); err != nil {
		t.Fatalf("no pude limpiar: %v", err)
	}
	if _, err := migrations.Apply(t.Context(), db); err != nil {
		t.Fatalf("no pude migrar: %v", err)
	}
	return db
}

func testStore(t *testing.T) *PgStore {
	t.Helper()
	return NewPgStore(testDB(t), t.Name())
}

func live(t *testing.T, store *PgStore) []Entry {
	t.Helper()
	entries, err := store.Live(t.Context())
	if err != nil {
		t.Fatalf("live: %v", err)
	}
	return entries
}

func list(t *testing.T, store *PgStore) []SessionMeta {
	t.Helper()
	sessions, err := store.List(t.Context())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return sessions
}

func archive(t *testing.T, store *PgStore) string {
	t.Helper()
	id, ok, err := store.Archive(t.Context())
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if !ok {
		t.Fatal("no archivó nada")
	}
	return id
}

func TestAStoreStartsEmptyAndScopesAreApart(t *testing.T) {
	db := testDB(t)
	store := NewPgStore(db, t.Name())
	other := NewPgStore(db, t.Name()+"/otro")

	if got := live(t, store); len(got) != 0 {
		t.Fatalf("arrancó con %+v", got)
	}
	if err := store.Append(t.Context(), []Entry{MessageEntry(sessionUser("hola"))}); err != nil {
		t.Fatal(err)
	}
	if got := live(t, other); len(got) != 0 {
		t.Fatalf("el otro scope ve %+v", got)
	}
	if sessions := list(t, other); len(sessions) != 0 {
		t.Fatalf("el otro scope lista %+v", sessions)
	}
	if id, ok, err := other.Archive(t.Context()); err != nil || ok {
		t.Fatalf("el otro scope archivó %q (%v)", id, err)
	}
}

func TestSaveAndAppendRoundTripTheTranscript(t *testing.T) {
	store := testStore(t)
	entries := append(sessionEntries(), MessageEntry(Message{
		Role:      "assistant",
		Content:   "voy a correr bash",
		Reasoning: "pienso ",
		ToolCalls: []ToolCall{{ID: "c1", Name: "bash", Arguments: `{"command":"ls"}`}},
	}))

	if err := store.Save(t.Context(), entries); err != nil {
		t.Fatal(err)
	}
	if got := live(t, store); !reflect.DeepEqual(got, entries) {
		t.Fatalf("round trip:\n%+v\n%+v", got, entries)
	}
	if err := store.Append(t.Context(), []Entry{MessageEntry(sessionUser("y ahora"))}); err != nil {
		t.Fatal(err)
	}
	got := live(t, store)
	if len(got) != len(entries)+1 || got[len(got)-1].Message.Content != "y ahora" {
		t.Fatalf("append: %+v", got)
	}
	if err := store.Save(t.Context(), entries[:1]); err != nil {
		t.Fatal(err)
	}
	if got := live(t, store); len(got) != 1 || got[0].Message.Content != "tarea original" {
		t.Fatalf("save no reemplazó: %+v", got)
	}
	if err := store.Save(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if got := live(t, store); len(got) != 1 {
		t.Fatalf("una entrada vacía no debería tocar nada: %+v", got)
	}
}

func TestArchiveKeepsTheSessionAndClearsTheLiveOne(t *testing.T) {
	store := testStore(t)
	if err := store.Save(t.Context(), []Entry{MessageEntry(sessionUser("first"))}); err != nil {
		t.Fatal(err)
	}
	id := archive(t, store)

	if got := live(t, store); len(got) != 0 {
		t.Fatalf("la viva sigue: %+v", got)
	}
	entries, ok, err := store.Load(t.Context(), id)
	if err != nil || !ok || len(entries) != 1 {
		t.Fatalf("load: %+v %v %v", entries, ok, err)
	}
	sessions := list(t, store)
	if len(sessions) != 1 || sessions[0].ID != id || sessions[0].Title != "first" || sessions[0].Turns != 1 {
		t.Fatalf("listado: %+v", sessions)
	}
	if _, ok, err := store.Load(t.Context(), "no-existe"); err != nil || ok {
		t.Fatalf("cargó una sesión que no existe: %v %v", ok, err)
	}
}

func TestArchivingNothingIsNotAnArchive(t *testing.T) {
	store := testStore(t)
	if id, ok, err := store.Archive(t.Context()); err != nil || ok {
		t.Fatalf("archivó %q (%v)", id, err)
	}
	if sessions := list(t, store); len(sessions) != 0 {
		t.Fatalf("listado: %+v", sessions)
	}
}

func TestResumeRewritesTheArchivedSession(t *testing.T) {
	store := testStore(t)
	if err := store.Save(t.Context(), []Entry{MessageEntry(sessionUser("first"))}); err != nil {
		t.Fatal(err)
	}
	id := archive(t, store)
	loaded, ok, err := store.Load(t.Context(), id)
	if err != nil || !ok {
		t.Fatalf("load: %v %v", ok, err)
	}
	if err := store.Save(t.Context(), []Entry{loaded[0], MessageEntry(sessionUser("second"))}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetResumeID(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	again := archive(t, store)
	if again != id {
		t.Fatalf("continuó en %q y no en %q", again, id)
	}
	if got := live(t, store); len(got) != 0 {
		t.Fatalf("la viva sigue: %+v", got)
	}
	if _, ok, err := store.ResumeID(t.Context()); err != nil || ok {
		t.Fatalf("el resume quedó pendiente: %v %v", ok, err)
	}
	sessions := list(t, store)
	if len(sessions) != 1 || sessions[0].ID != id || sessions[0].Turns != 2 {
		t.Fatalf("listado: %+v", sessions)
	}
	entries, _, err := store.Load(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	messages := ContextMessages(entries)
	if len(messages) != 2 || messages[1].Content != "second" {
		t.Fatalf("mensajes: %+v", messages)
	}
}

func TestResumeOfASessionThatWasNeverArchived(t *testing.T) {
	store := testStore(t)
	if err := store.Save(t.Context(), []Entry{MessageEntry(sessionUser("first"))}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetResumeID(t.Context(), "1790000000000"); err != nil {
		t.Fatal(err)
	}
	if id := archive(t, store); id != "1790000000000" {
		t.Fatalf("archivó en %q", id)
	}
	entries, ok, err := store.Load(t.Context(), "1790000000000")
	if err != nil || !ok || len(entries) != 1 {
		t.Fatalf("load: %+v %v %v", entries, ok, err)
	}
}

func TestContinueArchivedWritesBackIntoTheSession(t *testing.T) {
	store := testStore(t)
	if err := store.Save(t.Context(), []Entry{MessageEntry(sessionUser("first"))}); err != nil {
		t.Fatal(err)
	}
	id := archive(t, store)
	grown := []Entry{MessageEntry(sessionUser("first")), MessageEntry(sessionAssistant("second"))}
	if err := store.Save(t.Context(), grown); err != nil {
		t.Fatal(err)
	}
	ok, err := store.ContinueArchived(t.Context(), id, grown)
	if err != nil || !ok {
		t.Fatalf("continue: %v %v", ok, err)
	}
	if got := live(t, store); len(got) != 0 {
		t.Fatalf("la viva sigue: %+v", got)
	}
	entries, _, err := store.Load(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(entries, grown) {
		t.Fatalf("quedó %+v", entries)
	}
	sessions := list(t, store)
	if len(sessions) != 1 || sessions[0].ID != id || sessions[0].Turns != 1 {
		t.Fatalf("listado: %+v", sessions)
	}
	if ok, err := store.ContinueArchived(t.Context(), id, nil); err != nil || ok {
		t.Fatalf("sin entradas no debería escribir: %v %v", ok, err)
	}
}

func TestContinueArchivedLiveMovesTheTranscript(t *testing.T) {
	store := testStore(t)
	if err := store.Save(t.Context(), []Entry{MessageEntry(sessionUser("first"))}); err != nil {
		t.Fatal(err)
	}
	id := archive(t, store)
	if err := store.Save(t.Context(), []Entry{MessageEntry(sessionUser("first")), MessageEntry(sessionUser("second"))}); err != nil {
		t.Fatal(err)
	}
	ok, err := store.ContinueArchivedLive(t.Context(), id)
	if err != nil || !ok {
		t.Fatalf("continue: %v %v", ok, err)
	}
	if got := live(t, store); len(got) != 0 {
		t.Fatalf("la viva sigue: %+v", got)
	}
	entries, _, err := store.Load(t.Context(), id)
	if err != nil || len(entries) != 2 {
		t.Fatalf("load: %+v %v", entries, err)
	}
	if sessions := list(t, store); len(sessions) != 1 || sessions[0].Turns != 2 {
		t.Fatalf("listado: %+v", sessions)
	}
	if ok, err := store.ContinueArchivedLive(t.Context(), id); err != nil || ok {
		t.Fatalf("sin viva no hay nada que mover: %v %v", ok, err)
	}
}

func TestDiscardDropsTheLiveSessionAndTheResume(t *testing.T) {
	store := testStore(t)
	if err := store.Save(t.Context(), []Entry{MessageEntry(sessionUser("keep me not"))}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetResumeID(t.Context(), "abc"); err != nil {
		t.Fatal(err)
	}
	if err := store.Discard(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := live(t, store); len(got) != 0 {
		t.Fatalf("la viva sigue: %+v", got)
	}
	if _, ok, err := store.ResumeID(t.Context()); err != nil || ok {
		t.Fatalf("el resume sigue: %v %v", ok, err)
	}
}

func TestInvalidIdsAreIgnored(t *testing.T) {
	store := testStore(t)
	entry := []Entry{MessageEntry(sessionUser("x"))}
	for _, id := range []string{"", ".", "..", "a/b", `a\b`} {
		if err := store.SetResumeID(t.Context(), id); err != nil {
			t.Fatal(err)
		}
		if _, ok, err := store.ResumeID(t.Context()); err != nil || ok {
			t.Fatalf("%q quedó pendiente: %v %v", id, ok, err)
		}
		if _, ok, err := store.Load(t.Context(), id); err != nil || ok {
			t.Fatalf("%q cargó: %v %v", id, ok, err)
		}
		if ok, err := store.ContinueArchived(t.Context(), id, entry); err != nil || ok {
			t.Fatalf("%q escribió: %v %v", id, ok, err)
		}
		if ok, err := store.ContinueArchivedLive(t.Context(), id); err != nil || ok {
			t.Fatalf("%q movió: %v %v", id, ok, err)
		}
	}
	if sessions := list(t, store); len(sessions) != 0 {
		t.Fatalf("listado: %+v", sessions)
	}
}

func TestListOrdersNewestFirst(t *testing.T) {
	store := testStore(t)
	for _, title := range []string{"uno", "dos"} {
		if err := store.Save(t.Context(), []Entry{MessageEntry(sessionUser(title))}); err != nil {
			t.Fatal(err)
		}
		archive(t, store)
	}
	sessions := list(t, store)
	if len(sessions) != 2 || sessions[0].Title != "dos" || sessions[1].Title != "uno" {
		t.Fatalf("orden: %+v", sessions)
	}
}

func TestTitleAndTurnsTrackTheConversation(t *testing.T) {
	store := testStore(t)
	first := []Entry{MessageEntry(sessionUser("arreglá el compilador"))}
	if err := store.Append(t.Context(), first); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(t.Context(), []Entry{
		MessageEntry(sessionAssistant("ya está")),
		CompactionEntry("## Goal\narreglar", 10, 1, []Message{sessionUser("reciente")}),
	}); err != nil {
		t.Fatal(err)
	}
	id := archive(t, store)
	sessions := list(t, store)
	if len(sessions) != 1 || sessions[0].ID != id {
		t.Fatalf("listado: %+v", sessions)
	}
	if sessions[0].Turns != 2 {
		t.Fatalf("turnos: %+v", sessions[0])
	}
	if sessions[0].Title != "The conversation history before this point was compacted" {
		t.Fatalf("título: %q", sessions[0].Title)
	}
}

func TestTheLiveSessionIsNotLoadable(t *testing.T) {
	store := testStore(t)
	if err := store.Save(t.Context(), []Entry{MessageEntry(sessionUser("first"))}); err != nil {
		t.Fatal(err)
	}
	var id string
	err := store.db.QueryRowContext(t.Context(),
		"SELECT id FROM axe.sessions WHERE scope = $1 AND archived_at IS NULL", store.scope).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.Load(t.Context(), id); err != nil || ok {
		t.Fatalf("la viva se cargó: %v %v", ok, err)
	}
}

func TestSessionIdsDoNotCollideAcrossScopes(t *testing.T) {
	db := testDB(t)
	seen := map[string]bool{}
	for n := 0; n < 30; n++ {
		store := NewPgStore(db, fmt.Sprintf("%s/%d", t.Name(), n))
		if err := store.Append(t.Context(), []Entry{MessageEntry(sessionUser("x"))}); err != nil {
			t.Fatal(err)
		}
		var id string
		if err := db.QueryRowContext(t.Context(),
			"SELECT id FROM axe.sessions WHERE scope = $1", store.scope).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if seen[id] {
			t.Fatalf("id repetido: %s", id)
		}
		seen[id] = true
	}
}

func TestTwoWritersAppendWithoutLosingEntries(t *testing.T) {
	db := testDB(t)
	first := NewPgStore(db, t.Name())
	second := NewPgStore(db, t.Name())
	want := map[string]bool{}
	var wait sync.WaitGroup
	for index, store := range []*PgStore{first, second} {
		for writer := 0; writer < 4; writer++ {
			wait.Add(1)
			go func(store *PgStore, index, writer int) {
				defer wait.Done()
				for turn := 0; turn < 5; turn++ {
					content := fmt.Sprintf("%d/%d/%d", index, writer, turn)
					if err := store.Append(t.Context(), []Entry{MessageEntry(sessionUser(content))}); err != nil {
						t.Errorf("append %s: %v", content, err)
					}
				}
			}(store, index, writer)
		}
	}
	for index := range 2 {
		for writer := 0; writer < 4; writer++ {
			for turn := 0; turn < 5; turn++ {
				want[fmt.Sprintf("%d/%d/%d", index, writer, turn)] = true
			}
		}
	}
	wait.Wait()
	entries := live(t, first)
	if len(entries) != len(want) {
		t.Fatalf("quedaron %d entradas de %d", len(entries), len(want))
	}
	for _, entry := range entries {
		if !want[entry.Message.Content] {
			t.Fatalf("entrada ajena: %q", entry.Message.Content)
		}
		delete(want, entry.Message.Content)
	}
	id := archive(t, first)
	sessions := list(t, first)
	if len(sessions) != 1 || sessions[0].ID != id || sessions[0].Turns != 40 {
		t.Fatalf("listado: %+v", sessions)
	}
}
