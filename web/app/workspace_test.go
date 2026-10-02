package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/web/app"
	"github.com/3-lines-studio/goddard/web/app/apptest"
	"github.com/3-lines-studio/goddard/workspace"
)

func TestTheWorkspaceOfAnOwnerIsItsVolumeByDefault(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t))
	user := apptest.User(t, service)
	owner := chat.Owner{Kind: chat.OwnerUser, ID: user.ID}
	space, err := service.Workspace(t.Context(), owner)
	if err != nil {
		t.Fatalf("workspace: %v", err)
	}
	if space.Path != workspace.DefaultPath(service.Volumes, workspace.Owner{Kind: workspace.OwnerUser, ID: user.ID}) {
		t.Fatalf("el path quedó %q", space.Path)
	}
	dir, err := service.ProjectDir(t.Context(), chat.Project{Slug: "goddard", Owner: owner})
	if err != nil {
		t.Fatalf("project dir: %v", err)
	}
	if dir != filepath.Join(space.Path, "goddard") {
		t.Fatalf("el directorio quedó %q", dir)
	}
}

func TestSavingAWorkspaceWritesThePathAndTheSandbox(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t))
	user := apptest.User(t, service)
	owner := chat.Owner{Kind: chat.OwnerUser, ID: user.ID}
	path := filepath.Join(t.TempDir(), "volumen")
	if err := service.SaveWorkspace(t.Context(), owner, path, app.Sandbox{Addr: "127.0.0.1:2222", User: "root", Key: []byte("una-llave")}, user.ID); err != nil {
		t.Fatalf("save: %v", err)
	}
	space, err := service.Workspace(t.Context(), owner)
	if err != nil {
		t.Fatalf("workspace: %v", err)
	}
	if space.Path != path {
		t.Fatalf("el path quedó %q", space.Path)
	}
	sandbox, err := service.Sandbox(t.Context(), owner)
	if err != nil {
		t.Fatalf("sandbox: %v", err)
	}
	if err := sandbox.Ready(); err != nil {
		t.Fatalf("el sandbox no quedó listo: %v", err)
	}
	if string(sandbox.Key) != "una-llave" {
		t.Fatalf("la llave quedó %q", sandbox.Key)
	}
}

func TestSavingAWorkspaceLeavesWhatComesEmpty(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t))
	user := apptest.User(t, service)
	owner := chat.Owner{Kind: chat.OwnerUser, ID: user.ID}
	path := filepath.Join(t.TempDir(), "volumen")
	if err := service.SaveWorkspace(t.Context(), owner, path, app.Sandbox{Addr: "127.0.0.1:2222", User: "root", Key: []byte("una-llave")}, user.ID); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := service.SaveWorkspace(t.Context(), owner, path, app.Sandbox{Addr: "otra.local:22"}, user.ID); err != nil {
		t.Fatalf("save: %v", err)
	}
	sandbox, err := service.Sandbox(t.Context(), owner)
	if err != nil {
		t.Fatalf("sandbox: %v", err)
	}
	if err := sandbox.Ready(); err != nil {
		t.Fatalf("se perdió la llave: %v", err)
	}
	if sandbox.Addr != "otra.local:22" || sandbox.User != "root" {
		t.Fatalf("el sandbox quedó %+v", sandbox)
	}
}

func TestSavingAWorkspaceRefusesAPathThatIsNotAbsolute(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t))
	user := apptest.User(t, service)
	owner := chat.Owner{Kind: chat.OwnerUser, ID: user.ID}
	if err := service.SaveWorkspace(t.Context(), owner, "volumen", app.Sandbox{}, user.ID); !errors.Is(err, workspace.ErrPath) {
		t.Fatalf("un path relativo devolvió %v", err)
	}
}

func TestTheWorkspaceOfAnOrganizationIsItsOwn(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t))
	created := apptest.AnOrg(t, service, "La casa")
	path := filepath.Join(t.TempDir(), "la-casa")
	if err := service.SaveWorkspace(t.Context(), chat.Owner{Kind: chat.OwnerOrg, ID: created.ID}, path, app.Sandbox{Addr: "acme.local:22", User: "goddard", Key: []byte("la-de-acme")}, "berti"); err != nil {
		t.Fatalf("save: %v", err)
	}
	space, err := service.Workspace(t.Context(), chat.Owner{Kind: chat.OwnerOrg, ID: created.ID})
	if err != nil {
		t.Fatalf("workspace: %v", err)
	}
	if space.Path != path {
		t.Fatalf("el path quedó %q", space.Path)
	}
}

func TestRemovingAWorkspaceTakesThePathAndTheSandboxAway(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t))
	user := apptest.User(t, service)
	owner := chat.Owner{Kind: chat.OwnerUser, ID: user.ID}
	path := filepath.Join(t.TempDir(), "volumen")
	if err := service.SaveWorkspace(t.Context(), owner, path, app.Sandbox{Addr: "127.0.0.1:2222", User: "root", Key: []byte("una-llave")}, user.ID); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := service.RemoveWorkspace(t.Context(), owner, user.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	space, err := service.Workspace(t.Context(), owner)
	if err != nil {
		t.Fatalf("workspace: %v", err)
	}
	if want := workspace.DefaultPath(service.Volumes, workspace.Owner{Kind: workspace.OwnerUser, ID: user.ID}); space.Path != want {
		t.Fatalf("el path quedó %q", space.Path)
	}
	sandbox, err := service.Sandbox(t.Context(), owner)
	if err != nil {
		t.Fatalf("sandbox: %v", err)
	}
	if sandbox.Addr != "" || sandbox.User != "" || len(sandbox.Key) > 0 {
		t.Fatalf("el sandbox quedó %+v", sandbox)
	}
	if err := service.RemoveWorkspace(t.Context(), owner, user.ID); err != nil {
		t.Fatalf("sacar dos veces: %v", err)
	}
}

func TestCheckingAWorkspaceSaysWhetherTheVolumeIsThere(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t))
	user := apptest.User(t, service)
	owner := chat.Owner{Kind: chat.OwnerUser, ID: user.ID}
	volume := t.TempDir()
	sandbox := app.Sandbox{Addr: "127.0.0.1:22", User: "tester", Key: []byte("una-llave")}
	if err := service.SaveWorkspace(t.Context(), owner, volume, sandbox, user.ID); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := service.CheckWorkspace(t.Context(), owner); err != nil {
		t.Fatalf("el volumen está y dijo %v", err)
	}
	gone := filepath.Join(volume, "no-está")
	if err := service.SaveWorkspace(t.Context(), owner, gone, sandbox, user.ID); err != nil {
		t.Fatalf("save: %v", err)
	}
	err := service.CheckWorkspace(t.Context(), owner)
	if err == nil {
		t.Fatal("dijo que anda con un volumen que no está")
	}
	if !strings.Contains(err.Error(), gone) {
		t.Fatalf("no dijo cuál no ve: %v", err)
	}
	if _, err := os.Stat(gone); !os.IsNotExist(err) {
		t.Fatalf("lo creó: %v", err)
	}
}

func TestCheckingASandboxThatDoesNotAnswerSaysSo(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t))
	user := apptest.User(t, service)
	owner := chat.Owner{Kind: chat.OwnerUser, ID: user.ID}
	volume := t.TempDir()
	if err := service.SaveWorkspace(t.Context(), owner, volume, app.Sandbox{
		Addr: "127.0.0.1:1",
		User: "tester",
		Key:  []byte("no soy una llave"),
	}, user.ID); err != nil {
		t.Fatalf("save: %v", err)
	}
	service.Dialer = app.SSH{}
	err := service.CheckWorkspace(t.Context(), owner)
	if err == nil {
		t.Fatal("dijo que entró a una máquina que no contesta")
	}
	if !strings.Contains(err.Error(), "no pude entrar") {
		t.Fatalf("el error quedó %v", err)
	}
}

func TestCheckingASandboxWithNoKeySaysWhichOneIsMissing(t *testing.T) {
	if err := apptest.Service(t, apptest.Provider(t)).CheckWorkspace(t.Context(), chat.Owner{Kind: chat.OwnerOrg, ID: "acme"}); !errors.Is(err, app.ErrNoSandbox) {
		t.Fatalf("sin llave devolvió %v", err)
	}
}
