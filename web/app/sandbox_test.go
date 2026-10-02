package app_test

import (
	"errors"
	"testing"

	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/heimdall"
	"github.com/3-lines-studio/goddard/web/app"
	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func TestTheSandboxKeyIsTheOneOfThatSandboxOnly(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t))
	user := apptest.User(t, service)
	owner := heimdall.User(user.ID)
	for _, caso := range []struct{ project, env, value string }{
		{"goddard", app.SandboxEnv, "la-de-un-proyecto"},
		{app.SandboxProject, "prod", "la-de-producción"},
	} {
		if err := service.Heimdall.Set(t.Context(), owner, caso.project, caso.env, app.SandboxKeyName, caso.value, user.ID); err != nil {
			t.Fatalf("set: %v", err)
		}
	}
	if err := service.Heimdall.Set(t.Context(), owner, app.SandboxProject, app.SandboxEnv, app.SandboxKeyName, "una-llave", user.ID); err != nil {
		t.Fatalf("set: %v", err)
	}
	key, err := service.SandboxKey(t.Context(), chat.Owner{Kind: chat.OwnerUser, ID: user.ID})
	if err != nil {
		t.Fatalf("sandboxKey: %v", err)
	}
	if string(key) != "una-llave" {
		t.Fatalf("salió %q", key)
	}
}

func TestAnOrganizationHasItsOwnSandboxKey(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t))
	user := apptest.User(t, service)
	if err := service.Heimdall.Set(t.Context(), heimdall.User(user.ID), app.SandboxProject, app.SandboxEnv, app.SandboxKeyName, "la-mía", user.ID); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := service.Heimdall.Set(t.Context(), heimdall.Org("acme"), app.SandboxProject, app.SandboxEnv, app.SandboxKeyName, "la-de-acme", user.ID); err != nil {
		t.Fatalf("set: %v", err)
	}
	key, err := service.SandboxKey(t.Context(), chat.Owner{Kind: chat.OwnerOrg, ID: "acme"})
	if err != nil {
		t.Fatalf("sandboxKey: %v", err)
	}
	if string(key) != "la-de-acme" {
		t.Fatalf("salió %q", key)
	}
	if _, err := service.SandboxKey(t.Context(), chat.Owner{Kind: chat.OwnerUser, ID: "otro"}); !errors.Is(err, app.ErrNoSandboxKey) {
		t.Fatalf("un dueño sin llave devolvió %v", err)
	}
}

func TestAWorkspaceWithoutAKeyIsRefused(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t))
	user := apptest.User(t, service)
	if _, err := service.SandboxKey(t.Context(), chat.Owner{Kind: chat.OwnerUser, ID: user.ID}); !errors.Is(err, app.ErrNoSandboxKey) {
		t.Fatalf("sin llave devolvió %v", err)
	}
}
