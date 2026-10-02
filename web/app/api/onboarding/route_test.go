package onboarding

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/web/app"
	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func read(t *testing.T, cookie *http.Cookie) view {
	t.Helper()
	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/onboarding", nil, cookie))
	if recorder.Code != http.StatusOK {
		t.Fatalf("leer el onboarding contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	var body view
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("no pude leer la respuesta: %v", err)
	}
	return body
}

func TestTheOnboardingWantsASession(t *testing.T) {
	apptest.Route(t, apptest.Provider(t))
	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/onboarding", nil, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("leer sin sesión contestó %d", recorder.Code)
	}
}

func TestWithoutAMachineTheSetupIsNotDone(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	user, ok, err := service.Auth.User(t.Context(), apptest.TestEmail)
	if err != nil || !ok {
		t.Fatalf("user: %v (%v)", ok, err)
	}
	body := read(t, cookie)
	if body.Done {
		t.Fatalf("dijo que está listo sin máquina: %+v", body)
	}
	if body.Compute.HasKey || body.Compute.Addr != "" {
		t.Fatalf("trajo una máquina que nadie cargó: %+v", body.Compute)
	}
	if body.Compute.Path != filepath.Join(service.Volumes, user.ID) {
		t.Fatalf("el volumen por defecto quedó %q", body.Compute.Path)
	}
	if body.Model.Own || body.Model.HasKey {
		t.Fatalf("trajo un modelo que nadie cargó: %+v", body.Model)
	}
	if body.Model.House != service.Model {
		t.Fatalf("el de la casa quedó %q", body.Model.House)
	}
	if body.User.Email != apptest.TestEmail || body.User.Name != "berti" {
		t.Fatalf("el usuario quedó %+v", body.User)
	}
}

func TestAMachineAndAModelMakeTheSetupDone(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	user := apptest.User(t, service)
	owner := chat.Owner{Kind: chat.OwnerUser, ID: user.ID}
	if body := read(t, cookie); !body.Done {
		t.Fatalf("con máquina y el modelo de la casa sigue sin estar listo: %+v", body)
	}
	if err := service.SaveModel(t.Context(), owner, app.Model{
		Base: "https://mio.local/v1",
		Name: "mi-modelo",
		Key:  []byte("una-clave"),
	}, user.ID); err != nil {
		t.Fatalf("saveModel: %v", err)
	}
	body := read(t, cookie)
	if !body.Done || !body.Model.Own || !body.Model.HasKey {
		t.Fatalf("con el modelo propio quedó %+v", body)
	}
	if body.Model.Base != "https://mio.local/v1" || body.Model.Name != "mi-modelo" {
		t.Fatalf("el modelo quedó %+v", body.Model)
	}
}

func TestHalfAModelIsNotDone(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	user := apptest.User(t, service)
	if err := service.SaveModel(t.Context(), chat.Owner{Kind: chat.OwnerUser, ID: user.ID}, app.Model{
		Base: "https://mio.local/v1",
	}, user.ID); err != nil {
		t.Fatalf("saveModel: %v", err)
	}
	body := read(t, cookie)
	if body.Done {
		t.Fatalf("medio modelo dijo que está listo: %+v", body)
	}
	if !body.Model.Own {
		t.Fatalf("no vio el medio modelo: %+v", body.Model)
	}
}
