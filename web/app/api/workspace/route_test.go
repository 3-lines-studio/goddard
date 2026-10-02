package workspace

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/3-lines-studio/goddard/org"
	"github.com/3-lines-studio/goddard/web/app"
	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func read(t *testing.T, service *app.Service, cookie *http.Cookie, orgID string) view {
	t.Helper()
	target := "/api/workspace"
	if orgID != "" {
		target += "?org=" + orgID
	}
	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", target, nil, cookie))
	if recorder.Code != http.StatusOK {
		t.Fatalf("leer el workspace contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	var body view
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("no pude leer la respuesta: %v", err)
	}
	return body
}

func save(t *testing.T, cookie *http.Cookie, body map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/workspace", body, cookie))
	return recorder
}

func TestAWorkspaceWantsASession(t *testing.T) {
	apptest.Route(t, apptest.Provider(t))
	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/workspace", nil, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("leer sin sesión contestó %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/workspace", map[string]string{"path": "/volumes/uno"}, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("escribir sin sesión contestó %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	Delete(recorder, apptest.Request(t, "DELETE", "/api/workspace", nil, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("sacar sin sesión contestó %d", recorder.Code)
	}
}

func TestTheWorkspaceOfThePersonAskingComesAndGoes(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	path := filepath.Join(t.TempDir(), "volumen")

	if got := read(t, service, cookie, "").Path; got != filepath.Join(service.Volumes, apptest.User(t, service).ID) {
		t.Fatalf("el path por defecto quedó %q", got)
	}

	recorder := save(t, cookie, map[string]string{
		"path": path,
		"addr": "127.0.0.1:2222",
		"user": "root",
		"key":  "una-llave",
	})
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("guardar contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}

	got := read(t, service, cookie, "")
	if got.Path != path || got.Addr != "127.0.0.1:2222" || got.User != "root" {
		t.Fatalf("el workspace quedó %+v", got)
	}
	if !got.HasKey {
		t.Fatalf("dijo que no hay llave: %+v", got)
	}
	if got.Owner.Kind != "user" || got.Owner.ID == "" {
		t.Fatalf("el dueño quedó %+v", got.Owner)
	}
}

func TestTheKeyNeverComesBack(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	if recorder := save(t, cookie, map[string]string{
		"path": filepath.Join(t.TempDir(), "volumen"),
		"addr": "127.0.0.1:2222",
		"user": "root",
		"key":  "hunter2",
	}); recorder.Code != http.StatusNoContent {
		t.Fatalf("guardar contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/workspace", nil, cookie))
	if strings.Contains(recorder.Body.String(), "hunter2") {
		t.Fatalf("la llave volvió: %s", apptest.Text(t, recorder))
	}
}

func TestThePassphraseOfAKeyNeverComesBack(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	if recorder := save(t, cookie, map[string]string{
		"path":       filepath.Join(t.TempDir(), "volumen"),
		"addr":       "127.0.0.1:2222",
		"user":       "root",
		"key":        "una-llave",
		"passphrase": "una palabra",
	}); recorder.Code != http.StatusNoContent {
		t.Fatalf("guardar contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	if got := read(t, service, cookie, ""); !got.HasPassphrase {
		t.Fatalf("dijo que no hay passphrase: %+v", got)
	}
	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/workspace", nil, cookie))
	if strings.Contains(recorder.Body.String(), "una palabra") {
		t.Fatalf("la passphrase volvió: %s", apptest.Text(t, recorder))
	}
}

func TestTheFormCanSaveWithoutSendingTheKeyAgain(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	path := filepath.Join(t.TempDir(), "volumen")
	if recorder := save(t, cookie, map[string]string{
		"path": path, "addr": "127.0.0.1:2222", "user": "root", "key": "una-llave",
	}); recorder.Code != http.StatusNoContent {
		t.Fatalf("guardar contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	if recorder := save(t, cookie, map[string]string{
		"path": path, "addr": "otra.local:22",
	}); recorder.Code != http.StatusNoContent {
		t.Fatalf("guardar de nuevo contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	got := read(t, service, cookie, "")
	if got.Addr != "otra.local:22" || got.User != "root" || !got.HasKey {
		t.Fatalf("se perdió lo que ya estaba: %+v", got)
	}
}

func TestAWorkspaceNeedsAPathThatIsAbsolute(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	if recorder := save(t, cookie, map[string]string{"path": "volumen"}); recorder.Code != http.StatusBadRequest {
		t.Fatalf("un path relativo contestó %d", recorder.Code)
	}
}

func TestAMemberOfAnOrgSeesTheSandboxAndDoesNotWriteIt(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	ana := apptest.Session(t, service, "ana@ejemplo.com")
	created := apptest.AnOrg(t, service, "La casa")
	if err := service.Orgs.Add(t.Context(), created.ID, "ana@ejemplo.com", org.RoleMember, apptest.User(t, service).ID); err != nil {
		t.Fatalf("agregar: %v", err)
	}
	path := filepath.Join(t.TempDir(), "la-casa")
	if recorder := save(t, cookie, map[string]string{
		"org": created.ID, "path": path, "addr": "acme.local:22", "user": "goddard", "key": "la-de-acme",
	}); recorder.Code != http.StatusNoContent {
		t.Fatalf("guardar contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}

	got := read(t, service, ana, created.ID)
	if got.Path != path || got.Addr != "acme.local:22" || !got.HasKey {
		t.Fatalf("la miembro vio %+v", got)
	}
	if got.Owner.Kind != "org" || got.Owner.ID != created.ID {
		t.Fatalf("el dueño quedó %+v", got.Owner)
	}

	recorder := save(t, ana, map[string]string{"org": created.ID, "path": path, "addr": "de-ana.local:22"})
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("la miembro escribió: %d", recorder.Code)
	}
	if got := read(t, service, cookie, created.ID).Addr; got != "acme.local:22" {
		t.Fatalf("la dirección quedó %q", got)
	}
}

func TestAnAdminOfAnOrgWritesItsSandbox(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	ana := apptest.Session(t, service, "ana@ejemplo.com")
	created := apptest.AnOrg(t, service, "La casa")
	if err := service.Orgs.Add(t.Context(), created.ID, "ana@ejemplo.com", org.RoleAdmin, apptest.User(t, service).ID); err != nil {
		t.Fatalf("agregar: %v", err)
	}
	path := filepath.Join(t.TempDir(), "la-casa")
	if recorder := save(t, ana, map[string]string{
		"org": created.ID, "path": path, "addr": "acme.local:22", "user": "goddard", "key": "la-de-acme",
	}); recorder.Code != http.StatusNoContent {
		t.Fatalf("la admin no pudo guardar: %d %s", recorder.Code, apptest.Text(t, recorder))
	}
	if got := read(t, service, cookie, created.ID).Addr; got != "acme.local:22" {
		t.Fatalf("la dirección quedó %q", got)
	}
}

func TestTheWorkspaceOfAnOrgSomebodyIsNotInIsNotThere(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	ana := apptest.Session(t, service, "ana@ejemplo.com")
	created := apptest.AnOrg(t, service, "La casa")
	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/workspace?org="+created.ID, nil, ana))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("una ajena contestó %d", recorder.Code)
	}
	if recorder := save(t, ana, map[string]string{"org": created.ID, "path": "/volumes/x"}); recorder.Code != http.StatusNotFound {
		t.Fatalf("escribir una ajena contestó %d", recorder.Code)
	}
}

func TestRemovingTheWorkspaceOfThePersonAsking(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	path := filepath.Join(t.TempDir(), "volumen")
	if recorder := save(t, cookie, map[string]string{
		"path": path, "addr": "127.0.0.1:2222", "user": "root", "key": "una-llave",
	}); recorder.Code != http.StatusNoContent {
		t.Fatalf("guardar contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	recorder := httptest.NewRecorder()
	Delete(recorder, apptest.Request(t, "DELETE", "/api/workspace", nil, cookie))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("sacar contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	got := read(t, service, cookie, "")
	if got.Addr != "" || got.User != "" || got.HasKey {
		t.Fatalf("el sandbox quedó %+v", got)
	}
	if want := filepath.Join(service.Volumes, apptest.User(t, service).ID); got.Path != want {
		t.Fatalf("el path quedó %q", got.Path)
	}
	recorder = httptest.NewRecorder()
	Delete(recorder, apptest.Request(t, "DELETE", "/api/workspace", nil, cookie))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("sacar dos veces contestó %d", recorder.Code)
	}
}

func TestTheWorkspaceOfAnOrgIsNotForAMemberToRemove(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	ana := apptest.Session(t, service, "ana@ejemplo.com")
	created := apptest.AnOrg(t, service, "La casa")
	if err := service.Orgs.Add(t.Context(), created.ID, "ana@ejemplo.com", org.RoleMember, apptest.User(t, service).ID); err != nil {
		t.Fatalf("agregar: %v", err)
	}
	if recorder := save(t, cookie, map[string]string{
		"org": created.ID, "path": filepath.Join(t.TempDir(), "la-casa"), "addr": "acme.local:22", "user": "goddard", "key": "la-de-acme",
	}); recorder.Code != http.StatusNoContent {
		t.Fatalf("guardar contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	recorder := httptest.NewRecorder()
	Delete(recorder, apptest.Request(t, "DELETE", "/api/workspace?org="+created.ID, nil, ana))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("la miembro sacó el sandbox: %d", recorder.Code)
	}
	if got := read(t, service, cookie, created.ID); got.Addr != "acme.local:22" || !got.HasKey {
		t.Fatalf("la organización quedó %+v", got)
	}
	recorder = httptest.NewRecorder()
	Delete(recorder, apptest.Request(t, "DELETE", "/api/workspace?org="+created.ID, nil, cookie))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("el dueño no pudo sacarlo: %d %s", recorder.Code, apptest.Text(t, recorder))
	}
}
