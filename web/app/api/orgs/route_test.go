package orgs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func TestOrgsWantASession(t *testing.T) {
	apptest.Route(t, apptest.Provider(t))
	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/orgs", nil, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("listar sin sesión contestó %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/orgs", map[string]string{"name": "La casa"}, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("crear sin sesión contestó %d", recorder.Code)
	}
}

func TestCreatingAnOrgAndSeeingItWithTheRoleYouHave(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)

	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/orgs", map[string]string{"name": "La casa"}, cookie))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("crear contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	var created struct {
		ID   string `json:"id"`
		Slug string `json:"slug"`
		Role string `json:"role"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("no pude leer la respuesta: %v", err)
	}
	if created.Slug != "la-casa" || created.Role != "owner" || created.ID == "" {
		t.Fatalf("la organización quedó %+v", created)
	}

	recorder = httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/orgs", nil, cookie))
	if recorder.Code != http.StatusOK {
		t.Fatalf("listar contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	var body struct {
		Orgs []struct {
			ID   string `json:"id"`
			Slug string `json:"slug"`
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"orgs"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("no pude leer la respuesta: %v", err)
	}
	if len(body.Orgs) != 1 || body.Orgs[0].ID != created.ID || body.Orgs[0].Name != "La casa" || body.Orgs[0].Role != "owner" {
		t.Fatalf("la lista quedó %+v", body.Orgs)
	}
}

func TestAnOrgIsNotInTheListOfSomebodyOutOfIt(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	ana := apptest.Session(t, service, "ana@ejemplo.com")
	apptest.AnOrg(t, service, "La casa")

	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/orgs", nil, cookie))
	if recorder.Code != http.StatusOK {
		t.Fatalf("la del dueño contestó %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/orgs", nil, ana))
	var body struct {
		Orgs []any `json:"orgs"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("no pude leer la respuesta: %v", err)
	}
	if len(body.Orgs) != 0 {
		t.Fatalf("ana ve %+v sin estar", body.Orgs)
	}
}

func TestTheSameNameTwiceIsRefused(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	apptest.AnOrg(t, service, "La casa")

	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/orgs", map[string]string{"name": "La casa"}, cookie))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("la segunda contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
}

func TestDeletingAnOrgTakesItsProjectsWithIt(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	user := apptest.User(t, service)
	ana := apptest.Session(t, service, "ana@ejemplo.com")
	team := apptest.AnOrg(t, service, "La casa")
	if err := service.Orgs.Add(t.Context(), team.ID, "ana@ejemplo.com", "admin", user.ID); err != nil {
		t.Fatalf("no pude meter a ana: %v", err)
	}
	project, err := service.Chat.CreateProject(t.Context(), "de-la-org", chat.Owner{Kind: chat.OwnerOrg, ID: team.ID}, user.ID)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	mine := apptest.Thread(t, service)

	recorder := httptest.NewRecorder()
	Delete(recorder, apptest.Request(t, "DELETE", "/api/orgs?id="+team.ID, nil, ana))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("un admin la borró y contestó %d", recorder.Code)
	}
	if _, ok, err := service.Chat.Project(t.Context(), project.ID); err != nil || !ok {
		t.Fatalf("la negativa se llevó el proyecto (%v, %v)", ok, err)
	}

	recorder = httptest.NewRecorder()
	Delete(recorder, apptest.Request(t, "DELETE", "/api/orgs?id="+team.ID, nil, cookie))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("el dueño contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	if orgs, err := service.Orgs.Orgs(t.Context(), user.ID); err != nil || len(orgs) != 0 {
		t.Fatalf("le quedaron %+v (%v)", orgs, err)
	}
	if _, ok, err := service.Chat.Project(t.Context(), project.ID); err != nil || ok {
		t.Fatalf("el proyecto de la org siguió ahí (%v, %v)", ok, err)
	}
	if _, ok, err := service.Chat.Project(t.Context(), mine.ProjectID); err != nil || !ok {
		t.Fatalf("se llevó el proyecto propio (%v, %v)", ok, err)
	}

	recorder = httptest.NewRecorder()
	Delete(recorder, apptest.Request(t, "DELETE", "/api/orgs?id="+team.ID, nil, cookie))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("borrarla dos veces contestó %d", recorder.Code)
	}
}

func TestDeletingAnOrgWantsASession(t *testing.T) {
	apptest.Route(t, apptest.Provider(t))
	recorder := httptest.NewRecorder()
	Delete(recorder, apptest.Request(t, "DELETE", "/api/orgs?id=x", nil, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("sin sesión contestó %d", recorder.Code)
	}
}
