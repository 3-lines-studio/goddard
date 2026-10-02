package model

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/3-lines-studio/goddard/org"
	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func read(t *testing.T, cookie *http.Cookie, orgID string) view {
	t.Helper()
	target := "/api/model"
	if orgID != "" {
		target += "?org=" + orgID
	}
	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", target, nil, cookie))
	if recorder.Code != http.StatusOK {
		t.Fatalf("leer el modelo contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
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
	Post(recorder, apptest.Request(t, "POST", "/api/model", body, cookie))
	return recorder
}

func TestAModelWantsASession(t *testing.T) {
	apptest.Route(t, apptest.Provider(t))
	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/model", nil, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("leer sin sesión contestó %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/model", map[string]string{"name": "uno"}, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("escribir sin sesión contestó %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	Delete(recorder, apptest.Request(t, "DELETE", "/api/model", nil, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("sacar sin sesión contestó %d", recorder.Code)
	}
}

func TestTheModelOfThePersonAskingComesAndGoes(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)

	if got := read(t, cookie, ""); got.Own || got.House != service.Model {
		t.Fatalf("por defecto vio %+v", got)
	}

	if recorder := save(t, cookie, map[string]string{
		"base": "https://api.ejemplo.com", "name": "mi-modelo", "key": "una-clave",
	}); recorder.Code != http.StatusNoContent {
		t.Fatalf("guardar contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}

	got := read(t, cookie, "")
	if !got.Own || got.Base != "https://api.ejemplo.com" || got.Name != "mi-modelo" || !got.HasKey {
		t.Fatalf("con el propio vio %+v", got)
	}

	recorder := httptest.NewRecorder()
	Delete(recorder, apptest.Request(t, "DELETE", "/api/model", nil, cookie))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("sacar contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	if got := read(t, cookie, ""); got.Own || got.Name != "" || got.HasKey {
		t.Fatalf("después de sacarlo vio %+v", got)
	}
	recorder = httptest.NewRecorder()
	Delete(recorder, apptest.Request(t, "DELETE", "/api/model", nil, cookie))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("sacar dos veces contestó %d", recorder.Code)
	}
}

func TestTheModelOfAnOrgIsNotForAMemberToWrite(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	ana := apptest.Session(t, service, "ana@ejemplo.com")
	created := apptest.AnOrg(t, service, "La casa")
	if err := service.Orgs.Add(t.Context(), created.ID, "ana@ejemplo.com", org.RoleMember, apptest.User(t, service).ID); err != nil {
		t.Fatalf("agregar: %v", err)
	}
	if recorder := save(t, cookie, map[string]string{
		"org": created.ID, "base": "https://api.ejemplo.com", "name": "el-de-la-casa", "key": "la-clave",
	}); recorder.Code != http.StatusNoContent {
		t.Fatalf("el dueño no pudo guardar: %d %s", recorder.Code, apptest.Text(t, recorder))
	}
	if got := read(t, ana, created.ID); !got.Own || got.Name != "el-de-la-casa" || !got.HasKey {
		t.Fatalf("la miembro vio %+v", got)
	}
	if recorder := save(t, ana, map[string]string{"org": created.ID, "name": "el-de-ana"}); recorder.Code != http.StatusForbidden {
		t.Fatalf("la miembro escribió: %d", recorder.Code)
	}
	recorder := httptest.NewRecorder()
	Delete(recorder, apptest.Request(t, "DELETE", "/api/model?org="+created.ID, nil, ana))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("la miembro sacó el modelo: %d", recorder.Code)
	}
	if got := read(t, cookie, created.ID); got.Name != "el-de-la-casa" {
		t.Fatalf("la organización quedó %+v", got)
	}
}

func TestTheModelOfAnOrgSomebodyIsNotInIsNotThere(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	ana := apptest.Session(t, service, "ana@ejemplo.com")
	created := apptest.AnOrg(t, service, "La casa")
	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/model?org="+created.ID, nil, ana))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("una ajena contestó %d", recorder.Code)
	}
	if recorder := save(t, ana, map[string]string{"org": created.ID, "name": "el-de-ana"}); recorder.Code != http.StatusNotFound {
		t.Fatalf("escribir una ajena contestó %d", recorder.Code)
	}
}
