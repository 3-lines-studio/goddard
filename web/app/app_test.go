package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/3-lines-studio/goddard/auth"
	"github.com/3-lines-studio/goddard/axe"
	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/memo"
	"github.com/3-lines-studio/goddard/migrations"
	"github.com/3-lines-studio/goddard/prompt"
	"github.com/3-lines-studio/goddard/schedule"
	"github.com/3-lines-studio/goddard/skill"

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
	if _, err := db.ExecContext(t.Context(), "DROP SCHEMA IF EXISTS goddard, auth, chat, heimdall, axe, skill, memo, schedule CASCADE; DROP TABLE IF EXISTS public.schema_migrations"); err != nil {
		t.Fatalf("no pude limpiar: %v", err)
	}
	if _, err := migrations.Apply(t.Context(), db); err != nil {
		t.Fatalf("no pude migrar: %v", err)
	}
	return db
}

// provider answers with the turns it is given, one per request, the way the
// OpenAI-compatible servers do.
func provider(t *testing.T, turns ...[]string) *httptest.Server {
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
			_, _ = w.Write([]byte(sse(`{"choices":[{"delta":{},"finish_reason":"stop"}]}`)))
			return
		}
		_, _ = w.Write([]byte(sse(turns[served]...)))
		served++
	}))
	t.Cleanup(server.Close)
	return server
}

func sse(lines ...string) string {
	out := strings.Builder{}
	for _, line := range lines {
		out.WriteString("data: ")
		out.WriteString(line)
		out.WriteString("\n\n")
	}
	out.WriteString("data: [DONE]\n\n")
	return out.String()
}

func testService(t *testing.T, server *httptest.Server) *Service {
	t.Helper()
	db := testDB(t)
	return &Service{
		DB:        db,
		Chat:      chat.NewStore(db),
		Auth:      auth.NewStore(db),
		Memo:      memo.NewPgStore(db),
		Skill:     skill.NewPgStore(db),
		Schedule:  schedule.NewPgStore(db),
		Provider:  axe.NewOpenAI(server.URL, "k1"),
		Hub:       newHub(),
		Model:     "m1",
		Workspace: t.TempDir(),
		Viewer:    skill.Viewer{Org: "o1", User: "u1"},
		User:      "Don Berti",
		Assistant: "Jimmy",
		Language:  prompt.DefaultLanguage,
		Spec:      prompt.Default,
	}
}

func aThread(t *testing.T, service *Service) chat.Conversation {
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

// until waits for the turn to let the conversation go, which is the last thing
// answer does.
func until(t *testing.T, service *Service, id string) {
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

func bodies(t *testing.T, service *Service, id string) []string {
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

func eventOf(t *testing.T, body string) map[string]any {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("no pude leer %s: %v", body, err)
	}
	return parsed
}

func TestATurnLeavesTheLogWritten(t *testing.T) {
	service := testService(t, provider(t, []string{
		`{"choices":[{"delta":{"content":"cuarenta y dos"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`{"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2}}`,
	}))
	conversation := aThread(t, service)
	if err := service.Say(t.Context(), conversation.ID, "cuánto es 6*7", nil); err != nil {
		t.Fatalf("say: %v", err)
	}
	until(t, service, conversation.ID)
	got := bodies(t, service, conversation.ID)
	if len(got) != 3 {
		t.Fatalf("el log quedó %v", got)
	}
	first := eventOf(t, got[0])
	if first["event"] != "user" || !strings.Contains(first["text"].(string), "6*7") {
		t.Fatalf("primera línea: %s", got[0])
	}
	second := eventOf(t, got[1])
	if second["event"] != "assistant" || second["text"] != "cuarenta y dos" {
		t.Fatalf("segunda línea: %s", got[1])
	}
	if done := eventOf(t, got[2]); done["event"] != "done" {
		t.Fatalf("tercera línea: %s", got[2])
	}
	renamed, _, err := service.Chat.Conversation(t.Context(), conversation.ID)
	if err != nil {
		t.Fatalf("conversation: %v", err)
	}
	if renamed.Title == chat.NewTitle || !strings.Contains(renamed.Title, "6*7") {
		t.Fatalf("el título quedó %q", renamed.Title)
	}
}

func TestAToolGoesIntoTheLog(t *testing.T) {
	service := testService(t, provider(t,
		[]string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"bash","arguments":"{\"command\":\"echo hola\"}"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		},
		[]string{
			`{"choices":[{"delta":{"content":"listo"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		},
	))
	conversation := aThread(t, service)
	if err := service.Say(t.Context(), conversation.ID, "saludá", nil); err != nil {
		t.Fatalf("say: %v", err)
	}
	until(t, service, conversation.ID)
	lines := bodies(t, service, conversation.ID)
	if len(lines) != 5 {
		t.Fatalf("el log quedó %v", lines)
	}
	start := eventOf(t, lines[1])
	if start["event"] != "tool_start" || start["name"] != "bash" || start["id"] != "c1" {
		t.Fatalf("el tool_start quedó %v", start)
	}
	result := eventOf(t, lines[2])
	if result["event"] != "tool_result" || result["text"] != "hola\n" {
		t.Fatalf("el tool_result quedó %v", result)
	}
	if result["ms"] == nil {
		t.Fatal("el tool_result no dice cuánto tardó")
	}
	if assistant := eventOf(t, lines[3]); assistant["event"] != "assistant" || assistant["text"] != "listo" {
		t.Fatalf("el assistant quedó %v", assistant)
	}
}

func TestTheSecondTurnWaitsForTheFirst(t *testing.T) {
	service := testService(t, provider(t, []string{
		`{"choices":[{"delta":{"content":"ok"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}))
	conversation := aThread(t, service)
	if taken, err := service.Chat.Claim(t.Context(), conversation.ID, TurnLease); err != nil || !taken {
		t.Fatalf("no pude tomar la conversación: %v %v", taken, err)
	}
	if err := service.Say(t.Context(), conversation.ID, "otra vez", nil); err != ErrBusy {
		t.Fatalf("dio %v", err)
	}
}

func TestAConversationFromATransportIsReadOnly(t *testing.T) {
	service := testService(t, provider(t))
	project, err := service.Chat.CreateProject(t.Context(), "goddard", "u1")
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	from := "telegram"
	if _, err := service.Chat.CreateConversation(t.Context(), project.ID, "desde el bot", from, "u1"); err != nil {
		t.Fatalf("conversation: %v", err)
	}
	conversations, err := service.Chat.Conversations(t.Context(), project.ID)
	if err != nil {
		t.Fatalf("conversations: %v", err)
	}
	if err := service.Say(t.Context(), conversations[0].ID, "hola", nil); err != ErrReadOnly {
		t.Fatalf("dio %v", err)
	}
}

func TestALinkWorksOnceAndOpensASession(t *testing.T) {
	service := testService(t, provider(t))
	link, err := service.Login(t.Context(), "Berti@Ejemplo.com")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	user, session, err := service.Open(t.Context(), link)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if user.Email != "berti@ejemplo.com" || user.Name != "berti" {
		t.Fatalf("el usuario quedó %+v", user)
	}
	if _, _, err := service.Open(t.Context(), link); err == nil {
		t.Fatal("el link sirvió dos veces")
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: Cookie, Value: session})
	who, ok := service.Who(t.Context(), request)
	if !ok || who.ID != user.ID {
		t.Fatalf("la cookie dio %+v %v", who, ok)
	}
	service.Close(t.Context(), request)
	if _, ok := service.Who(t.Context(), request); ok {
		t.Fatal("la sesión sobrevivió al logout")
	}
}

func TestOnlyTheEmailsOnTheListMayAskForALink(t *testing.T) {
	service := testService(t, provider(t))
	service.Allowed = []string{"berti@ejemplo.com"}
	if _, err := service.Login(t.Context(), "otro@ejemplo.com"); err != ErrNotAllowed {
		t.Fatalf("dio %v", err)
	}
	if _, err := service.Login(t.Context(), "berti@ejemplo.com"); err != nil {
		t.Fatalf("el de la lista no pudo: %v", err)
	}
	if _, err := service.Login(t.Context(), "berti@ejemplo.com"); err != ErrTooSoon {
		t.Fatalf("dos veces seguidas dio %v", err)
	}
}

func TestATaskRunsInItsOwnThread(t *testing.T) {
	service := testService(t, provider(t, []string{
		`{"choices":[{"delta":{"content":"listo, Don Berti"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`{"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2}}`,
	}))
	aThread(t, service)
	task := schedule.Task{
		UserID:  "u1",
		Project: "goddard",
		Name:    "recordatorio",
		At:      "09:00",
		Prompt:  "avisale que corra los tests",
	}
	if err := service.Schedule.Add(t.Context(), task); err != nil {
		t.Fatalf("add: %v", err)
	}
	text, err := service.runTask(t.Context(), task)
	if err != nil {
		t.Fatalf("runTask: %v", err)
	}
	if text != "listo, Don Berti" {
		t.Fatalf("contestó %q", text)
	}
	thread, err := service.taskThread(t.Context(), task)
	if err != nil {
		t.Fatalf("taskThread: %v", err)
	}
	if thread.Title != "recordatorio" || thread.Source != chat.SourceSchedule {
		t.Fatalf("el hilo quedó %+v", thread)
	}
	got := bodies(t, service, thread.ID)
	if len(got) != 3 {
		t.Fatalf("el log quedó %v", got)
	}
	first := eventOf(t, got[0])
	if first["event"] != "user" || first["text"] != "avisale que corra los tests" {
		t.Fatalf("primera línea: %s", got[0])
	}
	second := eventOf(t, got[1])
	if second["event"] != "assistant" || second["text"] != "listo, Don Berti" {
		t.Fatalf("segunda línea: %s", got[1])
	}
	if done := eventOf(t, got[2]); done["event"] != "done" {
		t.Fatalf("tercera línea: %s", got[2])
	}
}

func TestATaskOfAMissingProjectDoesNotRun(t *testing.T) {
	service := testService(t, provider(t))
	task := schedule.Task{Project: "no-existe", Name: "suelta", At: "09:00", Prompt: "hola"}
	if _, err := service.runTask(t.Context(), task); err == nil {
		t.Fatal("corrió una tarea sin proyecto")
	}
}

func TestTheMessageCarriesTheFiles(t *testing.T) {
	got := messageOf("miralo", []attached{{Path: "files/c1/nota.txt", Mime: "text/plain"}})
	want := "miralo\n\nArchivos adjuntos:\n- files/c1/nota.txt (text/plain)\n"
	if got != want {
		t.Fatalf("el mensaje quedó %q", got)
	}
	if got := messageOf("solo texto", nil); got != "solo texto" {
		t.Fatalf("sin archivos quedó %q", got)
	}
	if got := messageOf("", []attached{{Path: "files/c1/foto.png", Mime: "image/png"}}); got != "Archivos adjuntos:\n- files/c1/foto.png (image/png)\n" {
		t.Fatalf("sin texto quedó %q", got)
	}
}

func TestAnAttachmentGoesToTheLogAndToTheWorkspace(t *testing.T) {
	service := testService(t, provider(t, []string{
		`{"choices":[{"delta":{"content":"la leí"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}))
	conversation := aThread(t, service)
	upload, err := service.Chat.PutUpload(t.Context(), conversation.ID, "nota.txt", "text/plain", []byte("hola"))
	if err != nil {
		t.Fatalf("putUpload: %v", err)
	}
	if err := service.Say(t.Context(), conversation.ID, "miralo", []string{upload.ID}); err != nil {
		t.Fatalf("say: %v", err)
	}
	until(t, service, conversation.ID)
	got := bodies(t, service, conversation.ID)
	if len(got) != 4 {
		t.Fatalf("el log quedó %v", got)
	}
	file := eventOf(t, got[1])
	if file["event"] != "file" || file["name"] != "nota.txt" || file["mime"] != "text/plain" {
		t.Fatalf("segunda línea: %s", got[1])
	}
	if file["id"] != upload.ID {
		t.Fatalf("el archivo del log no es el que subí: %s", got[1])
	}
	written, err := os.ReadFile(filepath.Join(service.Workspace, "files", conversation.ID, "nota.txt"))
	if err != nil {
		t.Fatalf("no encontré el archivo en el workspace: %v", err)
	}
	if string(written) != "hola" {
		t.Fatalf("el archivo quedó %q", written)
	}
}

func TestAMessageWithNoWordsAndNoFilesIsEmpty(t *testing.T) {
	service := testService(t, provider(t))
	conversation := aThread(t, service)
	if err := service.Say(t.Context(), conversation.ID, "  ", nil); err != ErrEmpty {
		t.Fatalf("un mensaje vacío dio %v", err)
	}
}

func TestTheAgentShowsAFileInTheThread(t *testing.T) {
	service := testService(t, provider(t,
		[]string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"send","arguments":"{\"path\":\"grafico.png\",\"caption\":\"el avance\"}"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		},
		[]string{
			`{"choices":[{"delta":{"content":"ahí va"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		}))
	conversation := aThread(t, service)
	image := append([]byte("\x89PNG\r\n\x1a\n"), []byte("lo que sea el resto")...)
	if err := os.WriteFile(filepath.Join(service.Workspace, "grafico.png"), image, 0o644); err != nil {
		t.Fatalf("no pude escribir el archivo: %v", err)
	}
	if err := service.Say(t.Context(), conversation.ID, "mostrame el gráfico", nil); err != nil {
		t.Fatalf("say: %v", err)
	}
	until(t, service, conversation.ID)
	got := bodies(t, service, conversation.ID)
	if len(got) != 6 {
		t.Fatalf("el log quedó %v", got)
	}
	file := eventOf(t, got[2])
	if file["event"] != "file" || file["name"] != "grafico.png" || file["mime"] != "image/png" {
		t.Fatalf("el evento quedó %s", got[2])
	}
	if file["caption"] != "el avance" {
		t.Fatalf("la caption quedó %s", got[2])
	}
	upload, ok, err := service.Chat.Upload(t.Context(), file["id"].(string))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if !ok || string(upload.Bytes) != string(image) {
		t.Fatalf("los bytes no son los del archivo")
	}
}

func TestTheAgentCannotShowWhatIsNotThere(t *testing.T) {
	service := testService(t, provider(t,
		[]string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"send","arguments":"{\"path\":\"no-existe.png\"}"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		},
		[]string{
			`{"choices":[{"delta":{"content":"no está"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		}))
	conversation := aThread(t, service)
	if err := service.Say(t.Context(), conversation.ID, "mostrame lo que no hay", nil); err != nil {
		t.Fatalf("say: %v", err)
	}
	until(t, service, conversation.ID)
	for _, body := range bodies(t, service, conversation.ID) {
		if event := eventOf(t, body); event["event"] == "file" {
			t.Fatalf("igual mandó %s", body)
		}
	}
}
