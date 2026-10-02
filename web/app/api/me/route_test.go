package me

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func patch(t *testing.T, cookie *http.Cookie, body map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	Patch(recorder, apptest.Request(t, "PATCH", "/api/me", body, cookie))
	return recorder
}

func TestTheNameWantsASession(t *testing.T) {
	apptest.Route(t, apptest.Provider(t))
	recorder := patch(t, nil, map[string]string{"name": "Berti"})
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("cambiar el nombre sin sesión contestó %d", recorder.Code)
	}
}

func TestTheNameIsTheOneSomebodyChose(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	user := apptest.User(t, service)
	if recorder := patch(t, cookie, map[string]string{"name": "  Berti  "}); recorder.Code != http.StatusNoContent {
		t.Fatalf("cambiar el nombre contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	found, ok, err := service.Auth.ByID(t.Context(), user.ID)
	if err != nil || !ok {
		t.Fatalf("byID: %v (%v)", ok, err)
	}
	if found.Name != "Berti" {
		t.Fatalf("el nombre quedó %q", found.Name)
	}
}

func TestAMissingOrEndlessNameIsRefused(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, apptest.TestEmail)
	user := apptest.User(t, service)
	for _, name := range []string{"", "   ", strings.Repeat("a", maxName+1)} {
		if recorder := patch(t, cookie, map[string]string{"name": name}); recorder.Code != http.StatusBadRequest {
			t.Fatalf("el nombre %q contestó %d", name, recorder.Code)
		}
	}
	found, _, err := service.Auth.ByID(t.Context(), user.ID)
	if err != nil {
		t.Fatalf("byID: %v", err)
	}
	if found.Name != "berti" {
		t.Fatalf("un nombre rechazado se guardó igual: %q", found.Name)
	}
}
