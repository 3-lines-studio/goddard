package agenda

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/3-lines-studio/goddard/schedule"
	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func aTask(name string) schedule.Task {
	return schedule.Task{UserID: "u1", Project: "goddard", Name: name, Every: "6h", Prompt: "reportá"}
}

func TestTheRoutesWantASession(t *testing.T) {
	apptest.Route(t, apptest.Provider(t))
	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/agenda?project=goddard", nil, nil))
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

func TestTheListIsTheTasksOfThatProjectWithTheirLastRun(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t, []string{
		`{"choices":[{"delta":{"content":"todo en orden"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}))
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	apptest.Thread(t, service)
	task := aTask("informe")
	if err := service.Schedule.Add(t.Context(), task); err != nil {
		t.Fatalf("no pude sembrar la tarea: %v", err)
	}
	if _, err := service.Agenda.RunNow(t.Context(), "u1", "goddard", "informe"); err != nil {
		t.Fatalf("runNow: %v", err)
	}

	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/agenda?project=goddard", nil, cookie))
	if recorder.Code != http.StatusOK {
		t.Fatalf("contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	var body struct {
		Tasks []struct {
			Name   string `json:"name"`
			Every  string `json:"every"`
			Paused bool   `json:"paused"`
			Last   *struct {
				OK   bool   `json:"ok"`
				Text string `json:"text"`
			} `json:"last"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("no pude leer la respuesta: %v", err)
	}
	if len(body.Tasks) != 1 || body.Tasks[0].Name != "informe" || body.Tasks[0].Every != "6h" {
		t.Fatalf("las tareas quedaron %+v", body.Tasks)
	}
	if body.Tasks[0].Last == nil || !body.Tasks[0].Last.OK || body.Tasks[0].Last.Text != "todo en orden" {
		t.Fatalf("el último run quedó %+v", body.Tasks[0].Last)
	}
}

func TestPauseAndDeleteAskForATaskThatIsThere(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	apptest.Thread(t, service)
	if err := service.Schedule.Add(t.Context(), aTask("informe")); err != nil {
		t.Fatalf("no pude sembrar la tarea: %v", err)
	}

	recorder := httptest.NewRecorder()
	Patch(recorder, apptest.Request(t, "PATCH", "/api/agenda", map[string]any{
		"project": "goddard", "name": "informe", "paused": true,
	}, cookie))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("pausar contestó %d", recorder.Code)
	}
	entry, err := service.Schedule.Get(t.Context(), "u1", "goddard", "informe")
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
	if _, err := service.Schedule.Get(t.Context(), "u1", "goddard", "informe"); err == nil {
		t.Fatal("la tarea siguió ahí")
	}
}
