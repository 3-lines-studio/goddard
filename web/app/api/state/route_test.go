package state

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/3-lines-studio/goddard/chat"
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

func TestStateLeavesTheAgendaThreadsOut(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	thread := apptest.Thread(t, service)
	if _, err := service.Chat.CreateConversation(t.Context(), thread.ProjectID, "memoria", chat.SourceSchedule, "u1"); err != nil {
		t.Fatalf("la conversación de la agenda: %v", err)
	}

	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/state", nil, cookie))
	if recorder.Code != http.StatusOK {
		t.Fatalf("contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	var body struct {
		Projects []struct {
			Conversations []struct {
				ID string `json:"id"`
			} `json:"conversations"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("no pude leer la respuesta: %v", err)
	}
	if len(body.Projects) != 1 || len(body.Projects[0].Conversations) != 1 {
		t.Fatalf("el sidebar quedó con %+v", body.Projects)
	}
	if body.Projects[0].Conversations[0].ID != thread.ID {
		t.Fatalf("quedó la conversación %q", body.Projects[0].Conversations[0].ID)
	}
}

func TestStateIsTheProjectsAndWhoIsAsking(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	user := apptest.User(t, service)
	thread := apptest.Thread(t, service)

	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/state", nil, cookie))
	if recorder.Code != http.StatusOK {
		t.Fatalf("contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	var body struct {
		User     string `json:"user"`
		Projects []struct {
			Slug          string `json:"slug"`
			Workspace     string `json:"workspace"`
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
	projects, err := service.Chat.Projects(t.Context(), chat.Owner{Kind: chat.OwnerUser, ID: user.ID}, nil)
	if err != nil {
		t.Fatalf("projects: %v", err)
	}
	dir, err := service.ProjectDir(t.Context(), projects[0])
	if err != nil {
		t.Fatalf("project dir: %v", err)
	}
	if len(projects) != 1 || body.Projects[0].Workspace != dir {
		t.Fatalf("el workspace del proyecto quedó %q", body.Projects[0].Workspace)
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

// El rail es un arbol: cada proyecto cuelga de su dueno, que es la persona o
// una de las organizaciones, y la respuesta trae las dos cosas.
func TestStateSaysWhoOwnsEveryProject(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	user := apptest.User(t, service)
	team := apptest.AnOrg(t, service, "La casa")
	if _, err := service.Chat.CreateProject(t.Context(), "de-la-org", chat.Owner{Kind: chat.OwnerOrg, ID: team.ID}, user.ID); err != nil {
		t.Fatalf("project: %v", err)
	}
	apptest.Thread(t, service)

	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/state", nil, cookie))
	if recorder.Code != http.StatusOK {
		t.Fatalf("contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	var body struct {
		Orgs []struct {
			ID   string `json:"id"`
			Slug string `json:"slug"`
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"orgs"`
		Projects []struct {
			Slug  string `json:"slug"`
			Owner struct {
				Kind string `json:"kind"`
				ID   string `json:"id"`
			} `json:"owner"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("no pude leer la respuesta: %v", err)
	}
	if len(body.Orgs) != 1 || body.Orgs[0].ID != team.ID || body.Orgs[0].Name != "La casa" || body.Orgs[0].Role != "owner" {
		t.Fatalf("las organizaciones quedaron %+v", body.Orgs)
	}
	if len(body.Projects) != 2 {
		t.Fatalf("los proyectos quedaron %+v", body.Projects)
	}
	owners := map[string]string{}
	for _, project := range body.Projects {
		owners[project.Slug] = project.Owner.Kind + ":" + project.Owner.ID
	}
	if owners["goddard"] != "user:"+user.ID {
		t.Fatalf("el propio quedó %+v", owners)
	}
	if owners["de-la-org"] != "org:"+team.ID {
		t.Fatalf("el de la org quedó %+v", owners)
	}
}
