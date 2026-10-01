package agenda

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/3-lines-studio/goddard/schedule"
	"github.com/3-lines-studio/goddard/web/app/api/agenda/read"
	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func aTask(userID, name string) schedule.Task {
	return schedule.Task{UserID: userID, Project: "goddard", Name: name, Every: "6h", Prompt: "reportá"}
}

func TestTheRoutesWantASession(t *testing.T) {
	apptest.Route(t, apptest.Provider(t))
	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/agenda", nil, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("listar sin sesión contestó %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	Patch(recorder, apptest.Request(t, "PATCH", "/api/agenda", map[string]any{"project": "goddard", "name": "x"}, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("pausar sin sesión contestó %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	Delete(recorder, apptest.Request(t, "DELETE", "/api/agenda?project=goddard&name=x", nil, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("borrar sin sesión contestó %d", recorder.Code)
	}
}

func TestTheListIsEveryTaskWithItsRuns(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t, []string{
		`{"choices":[{"delta":{"content":"todo en orden"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}))
	user := apptest.User(t, service)
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	apptest.Thread(t, service)
	if err := service.Schedule.Add(t.Context(), aTask(user.ID, "informe")); err != nil {
		t.Fatalf("no pude sembrar la tarea: %v", err)
	}
	otra := aTask(user.ID, "limpieza")
	otra.Project = "picsel"
	if err := service.Schedule.Add(t.Context(), otra); err != nil {
		t.Fatalf("no pude sembrar la otra tarea: %v", err)
	}
	if _, err := service.Agenda.RunNow(t.Context(), user.ID, "goddard", "informe"); err != nil {
		t.Fatalf("runNow: %v", err)
	}

	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/agenda", nil, cookie))
	if recorder.Code != http.StatusOK {
		t.Fatalf("contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	var body struct {
		Tasks []struct {
			Name    string `json:"name"`
			Project string `json:"project"`
			Every   string `json:"every"`
			Unread  int    `json:"unread"`
			Runs    []struct {
				TS   int64  `json:"ts"`
				Date string `json:"date"`
				OK   bool   `json:"ok"`
				Text string `json:"text"`
			} `json:"runs"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("no pude leer la respuesta: %v", err)
	}
	if len(body.Tasks) != 2 {
		t.Fatalf("la agenda quedó con %d tareas: %+v", len(body.Tasks), body.Tasks)
	}
	if body.Tasks[0].Name != "informe" || body.Tasks[0].Project != "goddard" {
		t.Fatalf("la primera quedó %+v", body.Tasks[0])
	}
	if body.Tasks[1].Name != "limpieza" || body.Tasks[1].Project != "picsel" || len(body.Tasks[1].Runs) != 0 {
		t.Fatalf("la segunda quedó %+v", body.Tasks[1])
	}
	if body.Tasks[0].Every != "6h" || len(body.Tasks[0].Runs) != 1 {
		t.Fatalf("el informe quedó %+v", body.Tasks[0])
	}
	if !body.Tasks[0].Runs[0].OK || body.Tasks[0].Runs[0].Text != "todo en orden" {
		t.Fatalf("la corrida quedó %+v", body.Tasks[0].Runs[0])
	}
	if body.Tasks[0].Unread != 1 {
		t.Fatalf("la corrida sin leer quedó en %d", body.Tasks[0].Unread)
	}
}

func TestReadingATaskAndReadingThemAll(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	user := apptest.User(t, service)
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	apptest.Thread(t, service)
	for _, task := range []schedule.Task{aTask(user.ID, "informe"), aTask(user.ID, "limpieza")} {
		if err := service.Schedule.Add(t.Context(), task); err != nil {
			t.Fatalf("no pude sembrar %q: %v", task.Name, err)
		}
		if _, err := service.DB.ExecContext(t.Context(),
			`INSERT INTO schedule.runs (user_id, project, name, run_ts, run_date, ms, ok, body)
			 VALUES ($1, 'goddard', $2, 100, '2026-10-01', 5, true, 'ok')`, user.ID, task.Name); err != nil {
			t.Fatalf("no pude sembrar la corrida de %q: %v", task.Name, err)
		}
	}

	recorder := httptest.NewRecorder()
	read.Post(recorder, apptest.Request(t, "POST", "/api/agenda/read", map[string]any{
		"project": "goddard", "name": "informe",
	}, cookie))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("leer una contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	if informe, _ := service.Schedule.Get(t.Context(), user.ID, "goddard", "informe"); informe.Unread != 0 {
		t.Fatalf("el informe quedó con %d sin leer", informe.Unread)
	}
	if limpieza, _ := service.Schedule.Get(t.Context(), user.ID, "goddard", "limpieza"); limpieza.Unread != 1 {
		t.Fatalf("la limpieza quedó con %d sin leer", limpieza.Unread)
	}

	recorder = httptest.NewRecorder()
	read.Post(recorder, apptest.Request(t, "POST", "/api/agenda/read", map[string]any{}, cookie))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("leer todas contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	entries, err := service.Schedule.ListAll(t.Context(), user.ID)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	for _, entry := range entries {
		if entry.Unread != 0 {
			t.Fatalf("%s quedó con %d sin leer", entry.Task.Name, entry.Unread)
		}
	}

	recorder = httptest.NewRecorder()
	read.Post(recorder, apptest.Request(t, "POST", "/api/agenda/read", map[string]any{
		"project": "goddard", "name": "no-existe",
	}, cookie))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("leer lo que no hay contestó %d", recorder.Code)
	}
}

func TestPauseAndDeleteAskForATaskThatIsThere(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	user := apptest.User(t, service)
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	apptest.Thread(t, service)
	if err := service.Schedule.Add(t.Context(), aTask(user.ID, "informe")); err != nil {
		t.Fatalf("no pude sembrar la tarea: %v", err)
	}

	recorder := httptest.NewRecorder()
	Patch(recorder, apptest.Request(t, "PATCH", "/api/agenda", map[string]any{
		"project": "goddard", "name": "informe", "paused": true,
	}, cookie))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("pausar contestó %d", recorder.Code)
	}
	entry, err := service.Schedule.Get(t.Context(), user.ID, "goddard", "informe")
	if err != nil || !entry.Task.Paused {
		t.Fatalf("la tarea quedó %+v (%v)", entry.Task, err)
	}

	recorder = httptest.NewRecorder()
	Patch(recorder, apptest.Request(t, "PATCH", "/api/agenda", map[string]any{
		"project": "goddard", "name": "no-existe", "paused": true,
	}, cookie))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("pausar lo que no hay contestó %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	Delete(recorder, apptest.Request(t, "DELETE", "/api/agenda?project=goddard&name=informe", nil, cookie))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("borrar contestó %d", recorder.Code)
	}
	if _, err := service.Schedule.Get(t.Context(), user.ID, "goddard", "informe"); err == nil {
		t.Fatal("la tarea siguió ahí")
	}
}
