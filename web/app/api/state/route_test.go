package state

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func TestStateWantsASession(t *testing.T) {
	apptest.Route(t, apptest.Provider(t))
	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/state", nil, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("sin sesión contestó %d", recorder.Code)
	}
}

func TestStateIsTheProjectsAndWhoIsAsking(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	thread := apptest.Thread(t, service)

	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/state", nil, cookie))
	if recorder.Code != http.StatusOK {
		t.Fatalf("contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	var body struct {
		User      string `json:"user"`
		Workspace string `json:"workspace"`
		Projects  []struct {
			Slug          string `json:"slug"`
			Conversations []struct {
				ID      string `json:"id"`
				Running bool   `json:"running"`
			} `json:"conversations"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("no pude leer la respuesta: %v", err)
	}
	if body.User != "berti@ejemplo.com" {
		t.Fatalf("el usuario quedó %q", body.User)
	}
	if body.Workspace != service.Workspace {
		t.Fatalf("el workspace quedó %q", body.Workspace)
	}
	if len(body.Projects) != 1 || body.Projects[0].Slug != "goddard" {
		t.Fatalf("los proyectos quedaron %+v", body.Projects)
	}
	if len(body.Projects[0].Conversations) != 1 || body.Projects[0].Conversations[0].ID != thread.ID {
		t.Fatalf("las conversaciones quedaron %+v", body.Projects[0].Conversations)
	}
	if body.Projects[0].Conversations[0].Running {
		t.Fatal("dijo que hay un turno corriendo y no hay ninguno")
	}
}
