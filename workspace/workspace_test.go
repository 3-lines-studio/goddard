package workspace

import "testing"

func TestAProjectHangsFromTheWorkspaceOfItsOwner(t *testing.T) {
	berti := Owner{Kind: OwnerUser, ID: "01M3"}
	space := New(berti)
	if space.Path != Volumes+"/01M3" {
		t.Fatalf("el path quedó %q", space.Path)
	}
	if space.ProjectDir("goddard") != Volumes+"/01M3/goddard" {
		t.Fatalf("el directorio del proyecto quedó %q", space.ProjectDir("goddard"))
	}
	if space.Reachable() {
		t.Fatal("dijo que un workspace sin sandbox es alcanzable")
	}
}

func TestAWorkspaceOfAnOrganizationIsTheSameThing(t *testing.T) {
	space := New(Owner{Kind: OwnerOrg, ID: "01M4"})
	if space.Path != Volumes+"/01M4" {
		t.Fatalf("el path quedó %q", space.Path)
	}
}

func TestAWorkspaceNeedsAnOwnerAndAnAbsolutePath(t *testing.T) {
	cases := map[string]Workspace{
		"sin dueño":            {Owner: Owner{}, Path: "/volumes/uno"},
		"dueño de nadie":       {Owner: Owner{Kind: "equipo", ID: "uno"}, Path: "/volumes/uno"},
		"sin id":               {Owner: Owner{Kind: OwnerUser}, Path: "/volumes/uno"},
		"path relativo":        {Owner: Owner{Kind: OwnerUser, ID: "uno"}, Path: "volumes/uno"},
		"path vacío":           {Owner: Owner{Kind: OwnerUser, ID: "uno"}, Path: ""},
		"sandbox sin user":     {Owner: Owner{Kind: OwnerUser, ID: "uno"}, Path: "/volumes/uno", Sandbox: Sandbox{Kind: SandboxSSH, Addr: "sandbox.local:22"}},
		"sandbox de otro tipo": {Owner: Owner{Kind: OwnerUser, ID: "uno"}, Path: "/volumes/uno", Sandbox: Sandbox{Kind: "kubernetes", Addr: "sandbox.local:22", User: "goddard"}},
	}
	for name, space := range cases {
		if err := space.Valid(); err == nil {
			t.Fatalf("%s: lo dio por válido", name)
		}
	}
}

func TestAWorkspaceWithASandboxIsReachable(t *testing.T) {
	space := New(Owner{Kind: OwnerUser, ID: "uno"})
	space.Sandbox = Sandbox{Kind: SandboxSSH, Addr: "127.0.0.1:2222", User: "root"}
	if err := space.Valid(); err != nil {
		t.Fatalf("no lo dio por válido: %v", err)
	}
	if !space.Reachable() {
		t.Fatal("dijo que no es alcanzable")
	}
}
