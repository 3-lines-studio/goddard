// Package workspace is where the projects of an owner live: the volume that
// holds them, in the `workspace` schema of the shared database (see
// migrations/).
//
// The workspace is the owner's — a person, or the organization the projects
// are shared with — and the project hangs from it by its slug. The path is
// stable on purpose: a project that moves leaves every file behind, and the
// files of another project are never on the way.
//
// The volume is persistent and the sandbox is not: the sandbox mounts the
// volume, runs the tools and keeps its own lifetime, so goddard connects to it
// and neither creates nor destroys it. Where that machine is and how to get
// into it is not here: it is a secret of the owner in heimdall, so it is not
// written down twice.
package workspace

import (
	"errors"
	"path/filepath"
	"strings"
)

type Owner struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

const (
	OwnerUser = "user"
	OwnerOrg  = "org"
)

// Volumes is where the volumes are mounted when nobody says otherwise.
const Volumes = "/volumes"

// Workspace is the config of one owner: where its projects live.
type Workspace struct {
	Owner Owner  `json:"owner"`
	Path  string `json:"path"`
}

var (
	ErrOwner = errors.New("ese workspace no es de un usuario ni de una organización")
	ErrPath  = errors.New("el path del workspace tiene que ser absoluto")
)

// DefaultPath is where the projects of an owner live when nobody said
// otherwise: the volume of the owner, under its id.
func DefaultPath(owner Owner) string {
	return filepath.Join(Volumes, owner.ID)
}

// New is the workspace of an owner with everything in its default.
func New(owner Owner) Workspace {
	return Workspace{Owner: owner, Path: DefaultPath(owner)}
}

// ProjectDir is the directory of a project: the workspace of the owner and the
// slug of the project inside it.
func (w Workspace) ProjectDir(slug string) string {
	return filepath.Join(w.Path, slug)
}

// Valid says whether the workspace can be written down.
func (w Workspace) Valid() error {
	if err := checkOwner(w.Owner); err != nil {
		return err
	}
	if !filepath.IsAbs(w.Path) {
		return ErrPath
	}
	return nil
}

func checkOwner(owner Owner) error {
	if owner.Kind != OwnerUser && owner.Kind != OwnerOrg {
		return ErrOwner
	}
	if strings.TrimSpace(owner.ID) == "" {
		return ErrOwner
	}
	return nil
}
