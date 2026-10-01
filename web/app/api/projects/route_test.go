package projects

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func TestTheRoutesWantASession(t *testing.T) {
	apptest.Route(t, apptest.Provider(t))
	for nombre, llamar := range map[string]func(http.ResponseWriter, *http.Request){
		"crear":     Post,
		"renombrar": Patch,
		"borrar":    Delete,
	} {
		recorder := httptest.NewRecorder()
		llamar(recorder, apptest.Request(t, "POST", "/api/projects", map[string]string{"name": "picsel"}, nil))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s sin sesión contestó %d", nombre, recorder.Code)
		}
	}
}

func TestCreateRenameAndDelete(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, "berti@ejemplo.com")

	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/projects", map[string]string{"name": "Picsel"}, cookie))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("crear contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	projects, err := service.Chat.Projects(t.Context())
	if err != nil || len(projects) != 1 {
		t.Fatalf("quedaron %d proyectos (%v)", len(projects), err)
	}
	if projects[0].Slug != "picsel" || projects[0].Name != "Picsel" {
		t.Fatalf("el proyecto quedó %+v", projects[0])
	}

	recorder = httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/projects", map[string]string{"name": "Picsel"}, cookie))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("el nombre repetido contestó %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	Patch(recorder, apptest.Request(t, "PATCH", "/api/projects", map[string]string{
		"id": projects[0].ID, "name": "el otro",
	}, cookie))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("renombrar contestó %d", recorder.Code)
	}
	renamed, _, err := service.Chat.Project(t.Context(), projects[0].ID)
	if err != nil || renamed.Name != "el otro" || renamed.Slug != "picsel" {
		t.Fatalf("el proyecto quedó %+v (%v)", renamed, err)
	}

	recorder = httptest.NewRecorder()
	Delete(recorder, apptest.Request(t, "DELETE", "/api/projects?id="+projects[0].ID, nil, cookie))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("borrar contestó %d", recorder.Code)
	}
	if projects, err := service.Chat.Projects(t.Context()); err != nil || len(projects) != 0 {
		t.Fatalf("quedaron %d proyectos (%v)", len(projects), err)
	}
}
