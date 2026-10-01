package members

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/3-lines-studio/goddard/org"
	"github.com/3-lines-studio/goddard/web/app"
	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func membersOf(t *testing.T, service *app.Service, cookie *http.Cookie, orgID string) []memberView {
	t.Helper()
	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/orgs/members?org="+orgID, nil, cookie))
	if recorder.Code != http.StatusOK {
		t.Fatalf("listar los miembros contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	var body struct {
		Members []memberView `json:"members"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("no pude leer la respuesta: %v", err)
	}
	return body.Members
}

func TestMembersWantASession(t *testing.T) {
	apptest.Route(t, apptest.Provider(t))
	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/orgs/members?org=x", nil, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("sin sesión contestó %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/orgs/members", map[string]string{"org": "x", "email": "a@b.c"}, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("sin sesión contestó %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	Patch(recorder, apptest.Request(t, "PATCH", "/api/orgs/members", map[string]string{"org": "x"}, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("sin sesión contestó %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	Delete(recorder, apptest.Request(t, "DELETE", "/api/orgs/members?org=x&user=y", nil, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("sin sesión contestó %d", recorder.Code)
	}
}

func TestTheOwnerBringsSomebodyInAndBothAreListed(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	apptest.Session(t, service, "ana@ejemplo.com")
	created := apptest.AnOrg(t, service, "La casa")

	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/orgs/members", map[string]string{
		"org": created.ID, "email": "ana@ejemplo.com", "role": org.RoleMember,
	}, cookie))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("agregar contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}

	people := membersOf(t, service, cookie, created.ID)
	if len(people) != 2 {
		t.Fatalf("quedaron %+v", people)
	}
	if people[0].Role != org.RoleOwner || people[0].Email != apptest.TestEmail {
		t.Fatalf("el primero quedó %+v", people[0])
	}
	if people[1].Role != org.RoleMember || people[1].Email != "ana@ejemplo.com" {
		t.Fatalf("el segundo quedó %+v", people[1])
	}
}

func TestSomebodyOutOfItSeesNothing(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	ana := apptest.Session(t, service, "ana@ejemplo.com")
	created := apptest.AnOrg(t, service, "La casa")

	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/orgs/members?org="+created.ID, nil, ana))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("la ajena contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	if people := membersOf(t, service, cookie, created.ID); len(people) != 1 {
		t.Fatalf("el dueño ve %+v", people)
	}
}

func TestAMemberBringsNobodyInAndAnOwnerChangesRoles(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	ana := apptest.Session(t, service, "ana@ejemplo.com")
	apptest.Session(t, service, "beto@ejemplo.com")
	created := apptest.AnOrg(t, service, "La casa")
	if err := service.Orgs.Add(t.Context(), created.ID, "ana@ejemplo.com", org.RoleMember, apptest.User(t, service).ID); err != nil {
		t.Fatalf("add: %v", err)
	}

	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/orgs/members", map[string]string{
		"org": created.ID, "email": "beto@ejemplo.com", "role": org.RoleMember,
	}, ana))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("un miembro agregó y contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}

	recorder = httptest.NewRecorder()
	Patch(recorder, apptest.Request(t, "PATCH", "/api/orgs/members", map[string]string{
		"org": created.ID, "user": apptest.User(t, service).ID, "role": "no-existe",
	}, cookie))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("un rol que no existe contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}

	people := membersOf(t, service, cookie, created.ID)
	recorder = httptest.NewRecorder()
	Patch(recorder, apptest.Request(t, "PATCH", "/api/orgs/members", map[string]string{
		"org": created.ID, "user": people[1].UserID, "role": org.RoleAdmin,
	}, cookie))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("el dueño cambió un rol y contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	if people = membersOf(t, service, cookie, created.ID); people[1].Role != org.RoleAdmin {
		t.Fatalf("el miembro quedó %+v", people[1])
	}
}

func TestTheLastOwnerDoesNotLeaveAndTheOthersDo(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	apptest.Session(t, service, "ana@ejemplo.com")
	created := apptest.AnOrg(t, service, "La casa")
	if err := service.Orgs.Add(t.Context(), created.ID, "ana@ejemplo.com", org.RoleOwner, apptest.User(t, service).ID); err != nil {
		t.Fatalf("add: %v", err)
	}

	people := membersOf(t, service, cookie, created.ID)
	if len(people) != 2 {
		t.Fatalf("quedaron %+v", people)
	}
	ana := ""
	for _, one := range people {
		if one.Email == "ana@ejemplo.com" {
			ana = one.UserID
		}
	}
	if ana == "" {
		t.Fatalf("ana no está: %+v", people)
	}
	recorder := httptest.NewRecorder()
	Delete(recorder, apptest.Request(t, "DELETE", "/api/orgs/members?org="+created.ID+"&user="+ana, nil, cookie))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("sacar a uno de dos dueños contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}

	people = membersOf(t, service, cookie, created.ID)
	if len(people) != 1 {
		t.Fatalf("quedaron %+v", people)
	}
	recorder = httptest.NewRecorder()
	Delete(recorder, apptest.Request(t, "DELETE", "/api/orgs/members?org="+created.ID+"&user="+people[0].UserID, nil, cookie))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("el último dueño se fue y contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	recorder = httptest.NewRecorder()
	Delete(recorder, apptest.Request(t, "DELETE", "/api/orgs/members?org="+created.ID+"&user="+ana, nil, cookie))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("sacar a quien ya no está contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
}
