package app_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost"

	"github.com/3-lines-studio/goddard/web/app"
	"github.com/3-lines-studio/goddard/web/app/apptest"
	"github.com/3-lines-studio/goddard/web/app/login"
)

func TestTheAppIsForWhoeverHasASession(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := app.Load(request)
	if !bifrost.IsRedirect(err) || !strings.HasSuffix(err.Error(), "redirect to /login") {
		t.Fatalf("sin cookie dio %v", err)
	}
	request.AddCookie(apptest.Session(t, service, apptest.TestEmail))
	if _, err := app.Load(request); err != nil {
		t.Fatalf("con cookie dio %v", err)
	}
}

func TestTheLoginIsForWhoeverHasNoSession(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	request := httptest.NewRequest(http.MethodGet, "/login", nil)
	if _, err := login.Load(request); err != nil {
		t.Fatalf("sin cookie dio %v", err)
	}
	request.AddCookie(apptest.Session(t, service, apptest.TestEmail))
	_, err := login.Load(request)
	if !bifrost.IsRedirect(err) || !strings.HasSuffix(err.Error(), "redirect to /") {
		t.Fatalf("con cookie dio %v", err)
	}
}
