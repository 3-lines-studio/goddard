package logout

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/3-lines-studio/goddard/web/app"
	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func TestPostTakesTheSessionAway(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	if !who(t, service, cookie) {
		t.Fatal("la cookie no servía antes de salir")
	}
	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/logout", nil, cookie))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("contestó %d", recorder.Code)
	}
	if who(t, service, cookie) {
		t.Fatal("la sesión sobrevivió al logout")
	}
	if got := recorder.Result().Cookies(); len(got) != 1 || got[0].MaxAge != -1 {
		t.Fatalf("la respuesta dejó %+v", got)
	}
}

func who(t *testing.T, service *app.Service, cookie *http.Cookie) bool {
	t.Helper()
	request := httptest.NewRequest("GET", "/", nil)
	request.AddCookie(cookie)
	_, ok := service.Who(t.Context(), request)
	return ok
}
