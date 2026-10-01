// Package apptest builds what a test of a route needs: a database of its own,
// the app over it and a provider that answers with what the test says. The
// routes reach the app through app.Current(), so a test hands it over with
// app.Start, the same door the binary comes through.
package apptest

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/3-lines-studio/goddard/axe"
	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/migrations"
	"github.com/3-lines-studio/goddard/prompt"
	"github.com/3-lines-studio/goddard/skill"
	"github.com/3-lines-studio/goddard/web/app"
)

const testLock = 0x676f6464544553

// Database is a database of the test's own: it drops every schema, applies the
// migrations and holds the advisory lock while the test runs, so two of them
// never fight over the same one.
func Database(t *testing.T) *sql.DB {
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

// Provider answers with the turns it is given, one per request, the way an
// OpenAI-compatible server does.
func Provider(t *testing.T, turns ...[]string) *httptest.Server {
	t.Helper()
	served := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("no pude leer el request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if served >= len(turns) {
			t.Errorf("pidió un turno de más: %d", served+1)
			_, _ = w.Write([]byte(SSE(`{"choices":[{"delta":{},"finish_reason":"stop"}]}`)))
			return
		}
		_, _ = w.Write([]byte(SSE(turns[served]...)))
		served++
	}))
	t.Cleanup(server.Close)
	return server
}

// SSE is the body of one turn of that provider.
func SSE(lines ...string) string {
	out := strings.Builder{}
	for _, line := range lines {
		out.WriteString("data: ")
		out.WriteString(line)
		out.WriteString("\n\n")
	}
	out.WriteString("data: [DONE]\n\n")
	return out.String()
}

// Service is the app over that database and that provider, the way the binary
// builds it.
func Service(t *testing.T, server *httptest.Server) *app.Service {
	t.Helper()
	built := app.New(Database(t), axe.NewOpenAI(server.URL, "k1"), t.TempDir())
	built.Model = "m1"
	built.Viewer = skill.Viewer{Org: "o1", User: "u1"}
	built.User = "Don Berti"
	built.Assistant = "Jimmy"
	built.Language = prompt.DefaultLanguage
	built.Spec = prompt.Default
	return built
}

// Route is the app a test of a route answers with: it starts it, so that the
// route reaches it through app.Current() the way it does in the binary, and
// hands it back for the test to write whatever it needs first.
func Route(t *testing.T, server *httptest.Server) *app.Service {
	t.Helper()
	service := Service(t, server)
	app.Start(service)
	return service
}

// Session is a cookie for an email that already came in.
func Session(t *testing.T, service *app.Service, email string) *http.Cookie {
	t.Helper()
	token, err := service.Login(t.Context(), email)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, session, err := service.Open(t.Context(), token); err == nil {
		return &http.Cookie{Name: app.Cookie, Value: session}
	} else {
		t.Fatalf("open: %v", err)
	}
	return nil
}

// Request is a request with a body, and with the cookie when there is one.
func Request(t *testing.T, method, target string, body any, cookie *http.Cookie) *http.Request {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("no pude armar el cuerpo: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, target, reader)
	request.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	return request
}

// Thread is a project with one conversation in it.
func Thread(t *testing.T, service *app.Service) chat.Conversation {
	t.Helper()
	project, err := service.Chat.CreateProject(t.Context(), "goddard", "u1")
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	conversation, err := service.Chat.CreateConversation(t.Context(), project.ID, "", "", "u1")
	if err != nil {
		t.Fatalf("conversation: %v", err)
	}
	return conversation
}

// Wait stops when the turn lets the conversation go, which is the last thing
// it does.
func Wait(t *testing.T, service *app.Service, id string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		found, ok, err := service.Chat.Conversation(t.Context(), id)
		if err != nil {
			t.Fatalf("conversation: %v", err)
		}
		if !ok || found.ClaimedUntil == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("el turno no terminó")
}

// Events is what the log of that thread has, each line as it was written.
func Events(t *testing.T, service *app.Service, id string) []string {
	t.Helper()
	events, err := service.Chat.Events(t.Context(), id, 0)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	out := []string{}
	for _, event := range events {
		out = append(out, string(event.Body))
	}
	return out
}

// Event is one line of that log, as what it says.
func Event(t *testing.T, body string) map[string]any {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("no pude leer %s: %v", body, err)
	}
	return parsed
}

// Say is a message the test writes, with the attachments it names.
func Say(t *testing.T, service *app.Service, conversationID, text string, uploads []string) {
	t.Helper()
	if err := service.Say(t.Context(), conversationID, text, uploads); err != nil {
		t.Fatalf("say: %v", err)
	}
	Wait(t, service, conversationID)
}

// Text is a line of the body of a response, for the tests that read one.
func Text(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	return strings.TrimSpace(recorder.Body.String())
}
