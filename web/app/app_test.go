package app_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/schedule"
	"github.com/3-lines-studio/goddard/web/app"
	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func TestATurnLeavesTheLogWritten(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t, []string{
		`{"choices":[{"delta":{"content":"cuarenta y dos"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`{"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2}}`,
	}))
	conversation := apptest.Thread(t, service)
	if err := service.Say(t.Context(), conversation.ID, apptest.User(t, service), "cuánto es 6*7", nil); err != nil {
		t.Fatalf("say: %v", err)
	}
	apptest.Wait(t, service, conversation.ID)
	got := apptest.Events(t, service, conversation.ID)
	if len(got) != 3 {
		t.Fatalf("el log quedó %v", got)
	}
	first := apptest.Event(t, got[0])
	if first["event"] != "user" || !strings.Contains(first["text"].(string), "6*7") {
		t.Fatalf("primera línea: %s", got[0])
	}
	second := apptest.Event(t, got[1])
	if second["event"] != "assistant" || second["text"] != "cuarenta y dos" {
		t.Fatalf("segunda línea: %s", got[1])
	}
	if done := apptest.Event(t, got[2]); done["event"] != "done" {
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
	service := apptest.Service(t, apptest.Provider(t,
		[]string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"bash","arguments":"{\"command\":\"echo hola\"}"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		},
		[]string{
			`{"choices":[{"delta":{"content":"listo"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		},
	))
	conversation := apptest.Thread(t, service)
	if err := service.Say(t.Context(), conversation.ID, apptest.User(t, service), "saludá", nil); err != nil {
		t.Fatalf("say: %v", err)
	}
	apptest.Wait(t, service, conversation.ID)
	lines := apptest.Events(t, service, conversation.ID)
	if len(lines) != 5 {
		t.Fatalf("el log quedó %v", lines)
	}
	start := apptest.Event(t, lines[1])
	if start["event"] != "tool_start" || start["name"] != "bash" || start["id"] != "c1" {
		t.Fatalf("el tool_start quedó %v", start)
	}
	result := apptest.Event(t, lines[2])
	if result["event"] != "tool_result" || result["text"] != "hola\n" {
		t.Fatalf("el tool_result quedó %v", result)
	}
	if result["ms"] == nil {
		t.Fatal("el tool_result no dice cuánto tardó")
	}
	if assistant := apptest.Event(t, lines[3]); assistant["event"] != "assistant" || assistant["text"] != "listo" {
		t.Fatalf("el assistant quedó %v", assistant)
	}
}

func TestTheSecondTurnWaitsForTheFirst(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t, []string{
		`{"choices":[{"delta":{"content":"ok"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}))
	conversation := apptest.Thread(t, service)
	if taken, err := service.Chat.Claim(t.Context(), conversation.ID, app.TurnLease); err != nil || !taken {
		t.Fatalf("no pude tomar la conversación: %v %v", taken, err)
	}
	if err := service.Say(t.Context(), conversation.ID, apptest.User(t, service), "otra vez", nil); err != app.ErrBusy {
		t.Fatalf("dio %v", err)
	}
}

func TestAConversationFromATransportIsReadOnly(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t))
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
	if err := service.Say(t.Context(), conversations[0].ID, apptest.User(t, service), "hola", nil); err != app.ErrReadOnly {
		t.Fatalf("dio %v", err)
	}
}

func TestALinkWorksOnceAndOpensASession(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t))
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
	request.AddCookie(&http.Cookie{Name: app.Cookie, Value: session})
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
	service := apptest.Service(t, apptest.Provider(t))
	service.Allowed = []string{"berti@ejemplo.com"}
	if _, err := service.Login(t.Context(), "otro@ejemplo.com"); err != app.ErrNotAllowed {
		t.Fatalf("dio %v", err)
	}
	if _, err := service.Login(t.Context(), "berti@ejemplo.com"); err != nil {
		t.Fatalf("el de la lista no pudo: %v", err)
	}
	if _, err := service.Login(t.Context(), "berti@ejemplo.com"); err != app.ErrTooSoon {
		t.Fatalf("dos veces seguidas dio %v", err)
	}
}

func TestATaskRunsInItsOwnThread(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t, []string{
		`{"choices":[{"delta":{"content":"listo, Don Berti"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`{"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2}}`,
	}))
	conversation := apptest.Thread(t, service)
	user := apptest.User(t, service)
	task := schedule.Task{
		UserID:  user.ID,
		Project: "goddard",
		Name:    "recordatorio",
		At:      "09:00",
		Prompt:  "avisale que corra los tests",
	}
	if err := service.Schedule.Add(t.Context(), task); err != nil {
		t.Fatalf("add: %v", err)
	}
	run, err := service.Agenda.RunNow(t.Context(), user.ID, "goddard", "recordatorio")
	if err != nil {
		t.Fatalf("runNow: %v", err)
	}
	if !run.OK || !strings.Contains(run.Text, "listo, Don Berti") {
		t.Fatalf("el run quedó %+v", run)
	}
	threads, err := service.Chat.Conversations(t.Context(), conversation.ProjectID)
	if err != nil {
		t.Fatalf("conversations: %v", err)
	}
	thread, found := chat.Conversation{}, false
	for _, one := range threads {
		if one.Title == "recordatorio" {
			thread, found = one, true
		}
	}
	if !found {
		t.Fatalf("la tarea no tiene hilo propio: %+v", threads)
	}
	if thread.Source != chat.SourceSchedule {
		t.Fatalf("el hilo quedó %q", thread.Source)
	}
	got := apptest.Events(t, service, thread.ID)
	if len(got) != 3 {
		t.Fatalf("el log quedó %v", got)
	}
	if first := apptest.Event(t, got[0]); first["event"] != "user" || first["text"] != "avisale que corra los tests" {
		t.Fatalf("primera línea: %s", got[0])
	}
	if second := apptest.Event(t, got[1]); second["event"] != "assistant" || second["text"] != "listo, Don Berti" {
		t.Fatalf("segunda línea: %s", got[1])
	}
	if done := apptest.Event(t, got[2]); done["event"] != "done" {
		t.Fatalf("tercera línea: %s", got[2])
	}
}

func TestATaskOfAMissingProjectLeavesTheErrorInItsRun(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t))
	user := apptest.User(t, service)
	task := schedule.Task{UserID: user.ID, Project: "no-existe", Name: "suelta", At: "09:00", Prompt: "hola"}
	if err := service.Schedule.Add(t.Context(), task); err != nil {
		t.Fatalf("add: %v", err)
	}
	run, err := service.Agenda.RunNow(t.Context(), user.ID, "no-existe", "suelta")
	if err != nil {
		t.Fatalf("runNow: %v", err)
	}
	if run.OK {
		t.Fatal("corrió una tarea de un proyecto que no existe")
	}
	if !strings.Contains(run.Text, "no existe") {
		t.Fatalf("el run quedó %q", run.Text)
	}
}

func TestAnAttachmentGoesToTheLogAndToTheWorkspace(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t, []string{
		`{"choices":[{"delta":{"content":"la leí"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}))
	conversation := apptest.Thread(t, service)
	upload, err := service.Chat.PutUpload(t.Context(), conversation.ID, "nota.txt", "text/plain", []byte("hola"))
	if err != nil {
		t.Fatalf("putUpload: %v", err)
	}
	if err := service.Say(t.Context(), conversation.ID, apptest.User(t, service), "miralo", []string{upload.ID}); err != nil {
		t.Fatalf("say: %v", err)
	}
	apptest.Wait(t, service, conversation.ID)
	got := apptest.Events(t, service, conversation.ID)
	if len(got) != 4 {
		t.Fatalf("el log quedó %v", got)
	}
	file := apptest.Event(t, got[1])
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
	service := apptest.Service(t, apptest.Provider(t))
	conversation := apptest.Thread(t, service)
	if err := service.Say(t.Context(), conversation.ID, apptest.User(t, service), "  ", nil); err != app.ErrEmpty {
		t.Fatalf("un mensaje vacío dio %v", err)
	}
}

func TestTheAgentShowsAFileInTheThread(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t,
		[]string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"send","arguments":"{\"path\":\"grafico.png\",\"caption\":\"el avance\"}"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		},
		[]string{
			`{"choices":[{"delta":{"content":"ahí va"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		}))
	conversation := apptest.Thread(t, service)
	image := append([]byte("\x89PNG\r\n\x1a\n"), []byte("lo que sea el resto")...)
	if err := os.WriteFile(filepath.Join(service.Workspace, "grafico.png"), image, 0o644); err != nil {
		t.Fatalf("no pude escribir el archivo: %v", err)
	}
	if err := service.Say(t.Context(), conversation.ID, apptest.User(t, service), "mostrame el gráfico", nil); err != nil {
		t.Fatalf("say: %v", err)
	}
	apptest.Wait(t, service, conversation.ID)
	got := apptest.Events(t, service, conversation.ID)
	if len(got) != 6 {
		t.Fatalf("el log quedó %v", got)
	}
	file := apptest.Event(t, got[2])
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
	service := apptest.Service(t, apptest.Provider(t,
		[]string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"send","arguments":"{\"path\":\"no-existe.png\"}"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		},
		[]string{
			`{"choices":[{"delta":{"content":"no está"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		}))
	conversation := apptest.Thread(t, service)
	if err := service.Say(t.Context(), conversation.ID, apptest.User(t, service), "mostrame lo que no hay", nil); err != nil {
		t.Fatalf("say: %v", err)
	}
	apptest.Wait(t, service, conversation.ID)
	for _, body := range apptest.Events(t, service, conversation.ID) {
		if event := apptest.Event(t, body); event["event"] == "file" {
			t.Fatalf("igual mandó %s", body)
		}
	}
}
