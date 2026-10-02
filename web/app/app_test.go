package app_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/org"
	"github.com/3-lines-studio/goddard/schedule"
	"github.com/3-lines-studio/goddard/skill"
	"github.com/3-lines-studio/goddard/web/app"
	"github.com/3-lines-studio/goddard/web/app/api/conversations"
	"github.com/3-lines-studio/goddard/web/app/api/events"
	"github.com/3-lines-studio/goddard/web/app/api/projects"
	"github.com/3-lines-studio/goddard/web/app/api/state"
	"github.com/3-lines-studio/goddard/web/app/api/turns"
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
	project, err := service.Chat.CreateProject(t.Context(), "goddard", chat.Owner{Kind: chat.OwnerUser, ID: apptest.User(t, service).ID}, apptest.User(t, service).ID)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	from := "telegram"
	if _, err := service.Chat.CreateConversation(t.Context(), project.ID, "desde el bot", from, apptest.User(t, service).ID); err != nil {
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

func TestWithoutAMailerTheLinkStaysThere(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t))
	link, err := service.Login(t.Context(), "berti@ejemplo.com")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := service.SendLink(t.Context(), "berti@ejemplo.com", link); err != app.ErrNoMailer {
		t.Fatalf("sin mailer dio %v", err)
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
		Owner:   schedule.Owner{Kind: schedule.KindUser, ID: user.ID},
		Project: "goddard",
		Name:    "recordatorio",
		At:      "09:00",
		Prompt:  "avisale que corra los tests",
	}
	if err := service.Schedule.Add(t.Context(), task); err != nil {
		t.Fatalf("add: %v", err)
	}
	run, err := service.Agenda.RunNow(t.Context(), service.AgendaOf(t.Context(), user), "goddard", "recordatorio")
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
	task := schedule.Task{Owner: schedule.Owner{Kind: schedule.KindUser, ID: user.ID}, Project: "no-existe", Name: "suelta", At: "09:00", Prompt: "hola"}
	if err := service.Schedule.Add(t.Context(), task); err != nil {
		t.Fatalf("add: %v", err)
	}
	run, err := service.Agenda.RunNow(t.Context(), service.AgendaOf(t.Context(), user), "no-existe", "suelta")
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
	userID := apptest.User(t, service).ID
	dir, err := service.ProjectDir(t.Context(), chat.Project{Slug: "goddard", Owner: chat.Owner{Kind: chat.OwnerUser, ID: userID}})
	if err != nil {
		t.Fatalf("project dir: %v", err)
	}
	written, err := os.ReadFile(filepath.Join(dir, "files", conversation.ID, "nota.txt"))
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
	dir, err := service.ProjectDir(t.Context(), chat.Project{Slug: "goddard", Owner: chat.Owner{Kind: chat.OwnerUser, ID: apptest.User(t, service).ID}})
	if err != nil {
		t.Fatalf("project dir: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("no pude armar el workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "grafico.png"), image, 0o644); err != nil {
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

func TestSomebodyElseSeesNeitherTheProjectNorTheThread(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t, []string{
		`{"choices":[{"delta":{"content":"lista"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	thread := apptest.Thread(t, service)
	apptest.Say(t, service, thread.ID, "lista", nil)

	otro := apptest.Session(t, service, "ana@ejemplo.com")
	recorder := httptest.NewRecorder()
	state.Get(recorder, apptest.Request(t, "GET", "/api/state", nil, otro))
	var body struct {
		Projects []struct {
			ID string `json:"id"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("no pude leer el estado: %v", err)
	}
	if len(body.Projects) != 0 {
		t.Fatalf("ana ve %d proyectos ajenos", len(body.Projects))
	}

	recorder = httptest.NewRecorder()
	events.Get(recorder, apptest.Request(t, "GET", "/api/events?conversation="+thread.ID, nil, otro))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("el hilo ajeno contestó %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	turns.Post(recorder, apptest.Request(t, "POST", "/api/turns", map[string]any{"conversation": thread.ID, "text": "hola"}, otro))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("escribir en un hilo ajeno contestó %d", recorder.Code)
	}

	// y el dueño sigue viendo el hilo por la misma ruta:
	recorder = httptest.NewRecorder()
	events.Get(recorder, apptest.Request(t, "GET", "/api/events?conversation="+thread.ID, nil, cookie))
	if recorder.Code != http.StatusOK {
		t.Fatalf("el dueño leyó el hilo y contestó %d", recorder.Code)
	}
}

func TestAProjectOfAnOrganizationBelongsToItsMembers(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t, []string{
		`{"choices":[{"delta":{"content":"hola"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	created := apptest.AnOrg(t, service, "La casa")
	ana := apptest.Session(t, service, "ana@ejemplo.com")
	if err := service.Orgs.Add(t.Context(), created.ID, "ana@ejemplo.com", org.RoleMember, apptest.User(t, service).ID); err != nil {
		t.Fatalf("add: %v", err)
	}

	recorder := httptest.NewRecorder()
	projects.Post(recorder, apptest.Request(t, "POST", "/api/projects", map[string]string{"name": "Compartido", "org": created.ID}, cookie))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("crear en la org contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	var project struct {
		ID    string `json:"id"`
		Owner struct {
			Kind string `json:"kind"`
			ID   string `json:"id"`
		} `json:"owner"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &project); err != nil {
		t.Fatal(err)
	}
	if project.Owner.Kind != "org" || project.Owner.ID != created.ID {
		t.Fatalf("el dueño quedó %+v", project.Owner)
	}

	// el proyecto es del usuario que lo creó en su propia lista:
	recorder = httptest.NewRecorder()
	state.Get(recorder, apptest.Request(t, "GET", "/api/state", nil, cookie))
	var own struct {
		Projects []struct {
			ID string `json:"id"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &own); err != nil {
		t.Fatal(err)
	}
	if len(own.Projects) != 1 {
		t.Fatalf("el dueño ve %+v", own.Projects)
	}

	// y no en la lista del miembro, porque su dueño es la org y el miembro entra por el hilo:
	recorder = httptest.NewRecorder()
	conversations.Post(recorder, apptest.Request(t, "POST", "/api/conversations", map[string]string{"project": project.ID}, cookie))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("el hilo de la org contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	var thread struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &thread); err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	events.Get(recorder, apptest.Request(t, "GET", "/api/events?conversation="+thread.ID, nil, ana))
	if recorder.Code != http.StatusOK {
		t.Fatalf("un miembro de la org no pudo leer el hilo y contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
}

// El turno de una tarea de la organización corre como la organización: ve las
// skills del equipo y no las de la persona, y el prompt la nombra a ella.
func TestATaskOfAnOrgRunsAsTheOrg(t *testing.T) {
	served := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		served <- body.Messages[0].Content
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(apptest.SSE(
			`{"choices":[{"delta":{"content":"listo"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		)))
	}))
	t.Cleanup(server.Close)

	service := apptest.Service(t, server)
	user := apptest.User(t, service)
	team := apptest.AnOrg(t, service, "Acme")
	if _, err := service.Chat.CreateProject(t.Context(), "goddard", chat.Owner{Kind: chat.OwnerOrg, ID: team.ID}, user.ID); err != nil {
		t.Fatalf("project: %v", err)
	}
	if err := service.Skill.Put(t.Context(), service.Viewer(t.Context(), user), skill.Skill{
		Meta: skill.Meta{Owner: skill.Owner{Kind: skill.User, ID: user.ID}, Name: "mia", Description: "sólo mía"},
		Body: "# mía",
	}); err != nil {
		t.Fatalf("no pude sembrar la skill de la persona: %v", err)
	}
	if err := service.Skill.Put(t.Context(), skill.Viewer{Orgs: []string{team.ID}, User: user.ID}, skill.Skill{
		Meta: skill.Meta{Owner: skill.Owner{Kind: skill.Org, ID: team.ID}, Name: "del-equipo", Description: "del equipo"},
		Body: "# del equipo",
	}); err != nil {
		t.Fatalf("no pude sembrar la skill de la org: %v", err)
	}

	task := schedule.Task{
		Owner:   schedule.Owner{Kind: schedule.KindOrg, ID: team.ID},
		Project: "goddard",
		Name:    "reporte",
		At:      "09:00",
		Prompt:  "reportá",
	}
	if err := service.Schedule.Add(t.Context(), task); err != nil {
		t.Fatalf("add: %v", err)
	}
	run, err := service.Agenda.RunNow(t.Context(), service.AgendaOf(t.Context(), user), "goddard", "reporte")
	if err != nil {
		t.Fatalf("runNow: %v", err)
	}
	if !run.OK {
		t.Fatalf("la corrida quedó %+v", run)
	}

	select {
	case system := <-served:
		if !strings.Contains(system, "del-equipo") {
			t.Fatalf("el prompt no trajo la skill de la org:\n%s", system)
		}
		if strings.Contains(system, "sólo mía") {
			t.Fatalf("el prompt trajo la skill de la persona:\n%s", system)
		}
		if !strings.Contains(system, "Acme") {
			t.Fatalf("el prompt no nombró a la organización:\n%s", system)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("el turno no llegó al modelo")
	}
}
