// Package skill keeps the skills of goddard in Postgres: named instructions the
// agent loads into its context when the task calls for them. Port of jimmy's
// `src/skill.rs`, with the tree of directories replaced by rows in the `skill`
// schema of the shared database (see migrations/).
//
// A skill belongs to one owner — the system, an organization or a user — and
// names the role of that owner it is for, empty meaning all of them. A viewer
// sees the system's skills, its organization's and its own, and when the name
// repeats the closest owner wins: a user's skill shadows the organization's,
// which shadows the system's. That is jimmy's local-shadows-builtin, through
// ownership instead of directory order.
//
// The system's skills are read only: they are installed by hand and nobody
// writes them through this package.
package skill

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// The kinds of owner a skill can have.
const (
	System = "system"
	Org    = "org"
	User   = "user"
)

// EmptyIndex is what the system prompt gets when there is nothing to load.
const EmptyIndex = "No hay ninguna instalada."

// ErrReadOnly comes back from a write aimed at a skill of the system.
var ErrReadOnly = errors.New("los skills del sistema son de sólo lectura")

// Owner is who a skill belongs to: the system, an organization or a user. The
// id is opaque, the caller's business, and empty for the system.
type Owner struct {
	Kind string
	ID   string
}

// Viewer is who is asking. It sees the system's skills, the ones of every
// organization it is in and its own, and the ones its role is allowed to see.
type Viewer struct {
	Orgs []string
	User string
	Role string
}

// Meta is a skill without its body, which is what a listing needs.
type Meta struct {
	Owner       Owner
	Role        string
	Name        string
	Description string
	UpdatedAt   int64
}

// Skill is the whole thing.
type Skill struct {
	Meta
	Body      string
	CreatedBy string
	CreatedAt int64
}

// Index renders the list the system prompt shows, in the same shape jimmy
// used: one `nombre — descripción` per line, in order.
func Index(skills []Meta) string {
	if len(skills) == 0 {
		return EmptyIndex
	}
	lines := make([]string, 0, len(skills))
	for _, skill := range skills {
		lines = append(lines, skill.Name+" — "+skill.Description)
	}
	return strings.Join(lines, "\n")
}

// Load renders a skill for the model: a heading and the body.
func Load(skill Skill) string {
	return fmt.Sprintf("Skill %s\n\n%s", skill.Name, skill.Body)
}

func validateName(name string) error {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return fmt.Errorf("nombre de skill inválido: %s", name)
	}
	return nil
}

// canWrite says whether this viewer may write a skill of that owner. The
// system's belong to nobody: not even an organization can touch them.
func canWrite(viewer Viewer, owner Owner) error {
	switch owner.Kind {
	case System:
		return ErrReadOnly
	case Org:
		if slices.Contains(viewer.Orgs, owner.ID) {
			return nil
		}
		return fmt.Errorf("el skill es de la organización %q y no de las tuyas", owner.ID)
	case User:
		if owner.ID == viewer.User {
			return nil
		}
		return fmt.Errorf("el skill es del usuario %q y no tuyo", owner.ID)
	}
	return fmt.Errorf("dueño desconocido: %q", owner.Kind)
}

// rank orders owners from the closest to the farthest: the one that wins when
// two skills share a name.
func rank(kind string) int {
	switch kind {
	case User:
		return 2
	case Org:
		return 1
	case System:
		return 0
	}
	return -1
}

func nowMs() int64 {
	return time.Now().UnixMilli()
}
