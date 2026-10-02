package login

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func TestPostNeverHandsTheLinkBack(t *testing.T) {
	apptest.Route(t, apptest.Provider(t))
	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/login", map[string]string{"email": "Berti@Ejemplo.com"}, nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin mailer contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	if strings.Contains(recorder.Body.String(), "/auth") {
		t.Fatalf("la respuesta llevó el link: %s", recorder.Body.String())
	}
}

func TestPostRejectsWhatItCannot(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/login", map[string]string{"email": "no-es-un-mail"}, nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("un mail que no es un mail contestó %d", recorder.Code)
	}

	service.Allowed = []string{"berti@ejemplo.com"}
	recorder = httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/login", map[string]string{"email": "otro@ejemplo.com"}, nil))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("afuera de la lista contestó %d", recorder.Code)
	}

	if _, err := service.Login(t.Context(), "berti@ejemplo.com"); err != nil {
		t.Fatalf("login: %v", err)
	}
	recorder = httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/login", map[string]string{"email": "berti@ejemplo.com"}, nil))
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("dos veces seguidas contestó %d", recorder.Code)
	}
}
