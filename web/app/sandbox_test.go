package app_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/heimdall"
	"github.com/3-lines-studio/goddard/web/app"
	"github.com/3-lines-studio/goddard/web/app/apptest"
)

// putSandbox loads a sandbox the way the app reads it: three secrets of the
// owner, under the project and the environment of the sandbox.
func putSandbox(t *testing.T, service *app.Service, owner heimdall.Owner, addr, user, key string) {
	t.Helper()
	for name, value := range map[string]string{
		app.SandboxAddr: addr,
		app.SandboxUser: user,
		app.SandboxKey:  key,
	} {
		if err := service.Heimdall.Set(t.Context(), owner, app.SandboxProject, app.SandboxEnv, name, value, "berti"); err != nil {
			t.Fatalf("set %s: %v", name, err)
		}
	}
}

func TestTheSandboxOfAnOwnerComesFromTheVault(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t))
	user := apptest.User(t, service)
	putSandbox(t, service, heimdall.User(user.ID), "127.0.0.1:2222", "root", "una-llave")
	found, err := service.Sandbox(t.Context(), chat.Owner{Kind: chat.OwnerUser, ID: user.ID})
	if err != nil {
		t.Fatalf("sandbox: %v", err)
	}
	if found.Addr != "127.0.0.1:2222" || found.User != "root" || string(found.Key) != "una-llave" {
		t.Fatalf("salió %+v", found)
	}
}

func TestAnOrganizationHasItsOwnSandbox(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t))
	user := apptest.User(t, service)
	putSandbox(t, service, heimdall.User(user.ID), "mio.local:22", "berti", "la-mía")
	putSandbox(t, service, heimdall.Org("acme"), "acme.local:22", "goddard", "la-de-acme")
	found, err := service.Sandbox(t.Context(), chat.Owner{Kind: chat.OwnerOrg, ID: "acme"})
	if err != nil {
		t.Fatalf("sandbox: %v", err)
	}
	if found.Addr != "acme.local:22" || found.User != "goddard" || string(found.Key) != "la-de-acme" {
		t.Fatalf("salió %+v", found)
	}
	if _, err := service.Sandbox(t.Context(), chat.Owner{Kind: chat.OwnerUser, ID: "otro"}); err != nil {
		t.Fatalf("leer un dueño sin sandbox devolvió %v", err)
	}
	other, err := service.Sandbox(t.Context(), chat.Owner{Kind: chat.OwnerUser, ID: "otro"})
	if err != nil {
		t.Fatalf("sandbox: %v", err)
	}
	if err := other.Ready(); !errors.Is(err, app.ErrNoSandbox) {
		t.Fatalf("un dueño sin sandbox quedó listo: %v", err)
	}
}

func TestASandboxSaysWhichOfTheThreeIsMissing(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t))
	for name, value := range map[string]string{
		app.SandboxAddr: "127.0.0.1:2222",
		app.SandboxUser: "root",
	} {
		if err := service.Heimdall.Set(t.Context(), heimdall.Org("acme"), app.SandboxProject, app.SandboxEnv, name, value, "berti"); err != nil {
			t.Fatalf("set %s: %v", name, err)
		}
	}
	sandbox, err := service.Sandbox(t.Context(), chat.Owner{Kind: chat.OwnerOrg, ID: "acme"})
	if err != nil {
		t.Fatalf("sandbox: %v", err)
	}
	ready := sandbox.Ready()
	if !errors.Is(ready, app.ErrNoSandbox) {
		t.Fatalf("sin llave devolvió %v", ready)
	}
	if got := ready.Error(); !strings.Contains(got, app.SandboxKey) {
		t.Fatalf("no dijo cuál falta: %q", got)
	}
}

func TestTheSandboxIsNotASecretOfAProject(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t))
	user := apptest.User(t, service)
	owner := heimdall.User(user.ID)
	putSandbox(t, service, owner, "127.0.0.1:2222", "root", "una-llave")
	for name, value := range map[string]string{
		app.SandboxAddr: "otro.local:22",
		app.SandboxUser: "otro",
		app.SandboxKey:  "otra-llave",
	} {
		if err := service.Heimdall.Set(t.Context(), owner, "goddard", app.SandboxEnv, name, value, "berti"); err != nil {
			t.Fatalf("set %s: %v", name, err)
		}
	}
	found, err := service.Sandbox(t.Context(), chat.Owner{Kind: chat.OwnerUser, ID: user.ID})
	if err != nil {
		t.Fatalf("sandbox: %v", err)
	}
	if found.Addr != "127.0.0.1:2222" || string(found.Key) != "una-llave" {
		t.Fatalf("leyó lo del proyecto: %+v", found)
	}
}
