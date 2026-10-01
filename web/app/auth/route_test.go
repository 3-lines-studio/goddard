package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/3-lines-studio/goddard/web/app"
	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func TestGetWantsAToken(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/auth", nil, cookie))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("sin token contestó %d", recorder.Code)
	}
}

func TestGetBurnsTheLinkAndGoesHome(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	link, err := service.Login(t.Context(), "berti@ejemplo.com")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/auth?token="+link, nil, nil))
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("el link bueno contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	response := recorder.Result()
	if got := response.Header.Get("Location"); got != "/" {
		t.Fatalf("fue a %q", got)
	}
	cookies := response.Cookies()
	if len(cookies) != 1 || cookies[0].Name != app.Cookie || cookies[0].Value == "" {
		t.Fatalf("dejó %+v", cookies)
	}
	if !cookies[0].HttpOnly {
		t.Fatal("la cookie salió sin HttpOnly")
	}

	recorder = httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/auth?token="+link, nil, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("el mismo link dos veces contestó %d", recorder.Code)
	}
}
