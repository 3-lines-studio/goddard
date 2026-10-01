package projects

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/3-lines-studio/goddard/chat"
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
	user := apptest.User(t, service)
	projects, err := service.Chat.Projects(t.Context(), chat.Owner{Kind: chat.OwnerUser, ID: user.ID}, nil)
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
	if projects, err := service.Chat.Projects(t.Context(), chat.Owner{Kind: chat.OwnerUser, ID: apptest.User(t, service).ID}, nil); err != nil || len(projects) != 0 {
		t.Fatalf("quedaron %d proyectos (%v)", len(projects), err)
	}
}

// El proyecto ajeno es invisible: renombrarlo o borrarlo contesta 404 y no
// toca nada.
func TestChangingAProjectThatIsNotYours(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	ana := apptest.Session(t, service, "ana@ejemplo.com")
	berti := apptest.User(t, service)
	project, err := service.Chat.CreateProject(t.Context(), "de-berti", chat.Owner{Kind: chat.OwnerUser, ID: berti.ID}, berti.ID)
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	recorder := httptest.NewRecorder()
	Patch(recorder, apptest.Request(t, "PATCH", "/api/projects", map[string]string{
		"id": project.ID, "name": "mío ahora",
	}, ana))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("renombrarlo contestó %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	Delete(recorder, apptest.Request(t, "DELETE", "/api/projects?id="+project.ID, nil, ana))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("borrarlo contestó %d", recorder.Code)
	}
	found, ok, err := service.Chat.Project(t.Context(), project.ID)
	if err != nil || !ok || found.Name != "de-berti" {
		t.Fatalf("el proyecto quedó %+v (%v, %v)", found, ok, err)
	}

	member := apptest.Session(t, service, "luz@ejemplo.com")
	luz := apptest.User(t, service)
	team := apptest.AnOrg(t, service, "La casa")
	if err := service.Orgs.Add(t.Context(), team.ID, "luz@ejemplo.com", "member", berti.ID); err != nil {
		t.Fatalf("no pude meter a luz: %v", err)
	}
	shared, err := service.Chat.CreateProject(t.Context(), "de-la-org", chat.Owner{Kind: chat.OwnerOrg, ID: team.ID}, berti.ID)
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	recorder = httptest.NewRecorder()
	Patch(recorder, apptest.Request(t, "PATCH", "/api/projects", map[string]string{
		"id": shared.ID, "name": "de todos",
	}, member))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("un miembro de la org contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	found, _, err = service.Chat.Project(t.Context(), shared.ID)
	if err != nil || found.Name != "de todos" {
		t.Fatalf("el de la org quedó %+v (%v)", found, err)
	}
	if luz.ID == "" {
		t.Fatal("luz no existe")
	}
}
