package workspace

import "testing"

func TestAProjectHangsFromTheWorkspaceOfItsOwner(t *testing.T) {
	space := Workspace{Owner: Owner{Kind: OwnerUser, ID: "01M3"}, Path: DefaultPath("/volumes", Owner{Kind: OwnerUser, ID: "01M3"})}
	if space.Path != Volumes+"/01M3" {
		t.Fatalf("el path quedó %q", space.Path)
	}
	if space.ProjectDir("goddard") != Volumes+"/01M3/goddard" {
		t.Fatalf("el directorio del proyecto quedó %q", space.ProjectDir("goddard"))
	}
}

func TestTheVolumeRootIsDeploymentConfig(t *testing.T) {
	if got := DefaultPath("/data/volumes", Owner{Kind: OwnerOrg, ID: "01M4"}); got != "/data/volumes/01M4" {
		t.Fatalf("el path quedó %q", got)
	}
}

func TestAWorkspaceNeedsAnOwnerAndAnAbsolutePath(t *testing.T) {
	cases := map[string]Workspace{
		"sin dueño":      {Owner: Owner{}, Path: "/volumes/uno"},
		"dueño de nadie": {Owner: Owner{Kind: "equipo", ID: "uno"}, Path: "/volumes/uno"},
		"sin id":         {Owner: Owner{Kind: OwnerUser}, Path: "/volumes/uno"},
		"path relativo":  {Owner: Owner{Kind: OwnerUser, ID: "uno"}, Path: "volumes/uno"},
		"path vacío":     {Owner: Owner{Kind: OwnerUser, ID: "uno"}, Path: ""},
	}
	for name, space := range cases {
		if err := space.Valid(); err == nil {
			t.Fatalf("%s: lo dio por válido", name)
		}
	}
}

func TestAWorkspaceWithAnAbsolutePathIsValid(t *testing.T) {
	space := Workspace{Owner: Owner{Kind: OwnerUser, ID: "uno"}, Path: "/volumes/uno"}
	if err := space.Valid(); err != nil {
		t.Fatalf("no lo dio por válido: %v", err)
	}
}
