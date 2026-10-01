package run

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/3-lines-studio/goddard/schedule"
	"github.com/3-lines-studio/goddard/web/app"
	"github.com/3-lines-studio/goddard/web/app/apptest"
)

// corrida espera la corrida de la tarea: la ruta la deja corriendo y contesta,
// así que lo que hay que mirar es el log.
func corrida(t *testing.T, service *app.Service) schedule.Entry {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		entry, err := service.Schedule.Get(t.Context(), "u1", "goddard", "reporte")
		if err == nil && len(entry.Runs) > 0 {
			return entry
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("la tarea no corrió")
	return schedule.Entry{}
}

func TestPostWantsASession(t *testing.T) {
	apptest.Route(t, apptest.Provider(t))
	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/agenda/run", map[string]string{"project": "goddard", "name": "x"}, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("sin sesión contestó %d", recorder.Code)
	}
}

func TestPostLeavesTheTaskRunningAndTheRunIsInItsLog(t *testing.T) {
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
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("contestar %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	entry := corrida(t, service)
	if !entry.Runs[0].OK || entry.Runs[0].Text != "el repo está limpio" {
		t.Fatalf("el run quedó %+v", entry.Runs[0])
	}
	if entry.Unread != 1 {
		t.Fatalf("la corrida quedó en %d sin leer", entry.Unread)
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
