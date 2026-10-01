package conversations

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
		llamar(recorder, apptest.Request(t, "POST", "/api/conversations", map[string]string{"project": "p"}, nil))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s sin sesión contestó %d", nombre, recorder.Code)
		}
	}
}

func TestCreateRenameAndDelete(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	thread := apptest.Thread(t, service)

	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/conversations", map[string]string{
		"project": thread.ProjectID,
		"title":   "la primera",
	}, cookie))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("crear contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}

	recorder = httptest.NewRecorder()
	Patch(recorder, apptest.Request(t, "PATCH", "/api/conversations", map[string]string{
		"id":    thread.ID,
		"title": "la segunda",
	}, cookie))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("renombrar contestó %d", recorder.Code)
	}
	renamed, _, err := service.Chat.Conversation(t.Context(), thread.ID)
	if err != nil || renamed.Title != "la segunda" {
		t.Fatalf("el título quedó %q (%v)", renamed.Title, err)
	}

	recorder = httptest.NewRecorder()
	Delete(recorder, apptest.Request(t, "DELETE", "/api/conversations?id="+thread.ID, nil, cookie))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("borrar contestó %d", recorder.Code)
	}
	if _, ok, err := service.Chat.Conversation(t.Context(), thread.ID); err != nil || ok {
		t.Fatalf("la conversación siguió ahí: %v %v", ok, err)
	}
}

func TestCreateWantsAProjectAndRenameWantsATitle(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	thread := apptest.Thread(t, service)
	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/conversations", map[string]string{
		"project": "no-existe",
	}, cookie))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("sin proyecto contestó %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	Patch(recorder, apptest.Request(t, "PATCH", "/api/conversations", map[string]string{
		"id": thread.ID,
	}, cookie))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("sin título contestó %d", recorder.Code)
	}
}
