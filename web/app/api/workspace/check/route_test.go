package check

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/org"
	"github.com/3-lines-studio/goddard/web/app"
	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func post(t *testing.T, cookie *http.Cookie, body map[string]string) (answer, int) {
	t.Helper()
	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/workspace/check", body, cookie))
	if recorder.Code != http.StatusOK {
		return answer{}, recorder.Code
	}
	var found answer
	if err := json.Unmarshal(recorder.Body.Bytes(), &found); err != nil {
		t.Fatalf("no pude leer la respuesta: %s", apptest.Text(t, recorder))
	}
	return found, recorder.Code
}

func sandbox() app.Sandbox {
	return app.Sandbox{Addr: "127.0.0.1:22", User: "tester", Key: []byte("una-llave")}
}

func TestTheCheckCanProveAMachineThatIsNotLoadedYet(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	user := apptest.User(t, service)
	owner := chat.Owner{Kind: chat.OwnerUser, ID: user.ID}
	if err := service.RemoveWorkspace(t.Context(), owner, user.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	found, code := post(t, cookie, map[string]string{
		"path": t.TempDir(), "addr": "127.0.0.1:22", "user": "tester", "key": "una-llave",
	})
	if code != http.StatusOK || !found.OK {
		t.Fatalf("probando una máquina sin cargarla contestó %d %+v", code, found)
	}
	after, err := service.Sandbox(t.Context(), owner)
	if err != nil {
		t.Fatalf("sandbox: %v", err)
	}
	if after.Addr != "" || len(after.Key) > 0 {
		t.Fatalf("el chequeo la cargó: %+v", after)
	}
}

func TestTheCheckWantsASession(t *testing.T) {
	apptest.Route(t, apptest.Provider(t))
	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/workspace/check", map[string]string{}, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("sin sesión contestó %d", recorder.Code)
	}
}

func TestTheCheckSaysWhetherTheSandboxAnswers(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	user := apptest.User(t, service)
	owner := chat.Owner{Kind: chat.OwnerUser, ID: user.ID}
	volume := t.TempDir()
	if err := service.SaveWorkspace(t.Context(), owner, volume, sandbox(), user.ID); err != nil {
		t.Fatalf("save: %v", err)
	}
	found, code := post(t, cookie, map[string]string{})
	if code != http.StatusOK || !found.OK {
		t.Fatalf("con el volumen puesto contestó %d %+v", code, found)
	}

	gone := volume + "/no-está"
	if err := service.SaveWorkspace(t.Context(), owner, gone, sandbox(), user.ID); err != nil {
		t.Fatalf("save: %v", err)
	}
	found, code = post(t, cookie, map[string]string{})
	if code != http.StatusOK || found.OK {
		t.Fatalf("sin el volumen contestó %d %+v", code, found)
	}
	if !strings.Contains(found.Message, gone) {
		t.Fatalf("no dijo cuál no ve: %q", found.Message)
	}
}

func TestTheWorstThatCanHappenIsAnAnswerThatSaysNo(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	found, code := post(t, cookie, map[string]string{})
	if code != http.StatusOK || found.OK {
		t.Fatalf("sin sandbox contestó %d %+v", code, found)
	}
	if !strings.Contains(found.Message, "no tiene sandbox") {
		t.Fatalf("no dijo qué falta: %q", found.Message)
	}
}

func TestTheCheckOfAnOrgIsNotForWhoeverIsNotInIt(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	ana := apptest.Session(t, service, "ana@ejemplo.com")
	created := apptest.AnOrg(t, service, "La casa")
	if _, code := post(t, ana, map[string]string{"org": created.ID}); code != http.StatusNotFound {
		t.Fatalf("una ajena contestó %d", code)
	}
}

func TestAMemberChecksTheSandboxOfTheOrg(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	user := apptest.User(t, service)
	ana := apptest.Session(t, service, "ana@ejemplo.com")
	created := apptest.AnOrg(t, service, "La casa")
	if err := service.Orgs.Add(t.Context(), created.ID, "ana@ejemplo.com", org.RoleMember, user.ID); err != nil {
		t.Fatalf("agregar: %v", err)
	}
	owner := chat.Owner{Kind: chat.OwnerOrg, ID: created.ID}
	if err := service.SaveWorkspace(t.Context(), owner, t.TempDir(), sandbox(), user.ID); err != nil {
		t.Fatalf("save: %v", err)
	}
	found, code := post(t, ana, map[string]string{"org": created.ID})
	if code != http.StatusOK || !found.OK {
		t.Fatalf("la miembro contestó %d %+v", code, found)
	}
}
