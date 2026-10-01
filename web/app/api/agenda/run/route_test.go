package run

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/3-lines-studio/goddard/schedule"
	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func TestPostWantsASession(t *testing.T) {
	apptest.Route(t, apptest.Provider(t))
	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/agenda/run", map[string]string{"project": "goddard", "name": "x"}, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("sin sesión contestó %d", recorder.Code)
	}
}

func TestPostRunsTheTaskAndAnswersWhatItSaid(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t, []string{
		`{"choices":[{"delta":{"content":"el repo está limpio"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}))
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	apptest.Thread(t, service)
	task := schedule.Task{UserID: "u1", Project: "goddard", Name: "reporte", At: "09:00", Paused: true, Prompt: "reportá"}
	if err := service.Schedule.Add(t.Context(), task); err != nil {
		t.Fatalf("no pude sembrar la tarea: %v", err)
	}

	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/agenda/run", map[string]string{
		"project": "goddard", "name": "reporte",
	}, cookie))
	if recorder.Code != http.StatusOK {
		t.Fatalf("contestar %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	var body struct {
		OK   bool   `json:"ok"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("no pude leer la respuesta: %v", err)
	}
	if !body.OK || body.Text != "el repo está limpio" {
		t.Fatalf("el run quedó %+v", body)
	}
}

func TestPostSaysWhenTheTaskIsNotThere(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/agenda/run", map[string]string{
		"project": "goddard", "name": "no-existe",
	}, cookie))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("contestó %d", recorder.Code)
	}
}
