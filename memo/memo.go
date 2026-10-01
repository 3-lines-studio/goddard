// Package memo keeps the facts of goddard in Postgres: the small durable
// things the agent remembers about itself and about the project it is working
// on. Port of jimmy's `src/memo.rs`, with the tree of files replaced by rows in
// the `memo` schema of the shared database (see migrations/).
//
// A fact is a key, a kind from a short list, a body and the day it was last
// touched. The key says where it belongs: `usuario` is a general fact, and
// `jimmy/telemetria` one of the project `jimmy`. A fact updated replaces the
// old one instead of piling up next to it, because the key is stable.
//
// What the prompt gets is Render: every general fact plus the newest of the
// project in hand. The rest stays in the store and comes back when the topic
// comes back.
package memo

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"
)

// ProjectEntries is how many facts of the project in hand go into the prompt.
// The rest waits in the store: nothing is evicted by a cap.
const ProjectEntries = 2

// Kinds are the kinds a fact can have. The list is short on purpose: with an
// open vocabulary every entry invented its own and the kind stopped meaning
// anything.
var Kinds = []string{
	"decision",
	"estado",
	"medicion",
	"bugfix",
	"herramienta",
	"identidad",
	"proyecto",
	"plataforma",
}

// Owner is who a memory belongs to: a person, or the organization a project
// belongs to. The pair is the one the rest of goddard keeps.
type Owner struct {
	Kind string
	ID   string
}

// The kinds of owner, the same two the rest of goddard knows.
const (
	KindUser = "user"
	KindOrg  = "org"
)

// Scope is whose memory is being read or written. The general facts — the ones
// whose key has no slash — belong to the person asking; the ones of a project
// belong to whoever owns it, which is a person or an organization.
type Scope struct {
	User    Owner
	Project Owner
	Slug    string
}

// whereOf is who a key belongs to, and in what project: a key with a slash is
// of a project, and one without is general.
func (s Scope) whereOf(key string) (Owner, string) {
	if project := projectOf(key); project != "" {
		return s.Project, project
	}
	return s.User, ""
}

// Fact is one thing the agent remembers. Date is the day it was last touched,
// in days since the epoch, the way jimmy kept it.
type Fact struct {
	Project string
	Key     string
	Kind    string
	Body    string
	Date    int64
}

// Revision is a fact as it was on one of the days it changed. It is the level
// two of jimmy's store: what `show` falls back to when the fact is gone.
type Revision struct {
	Project  string
	Key      string
	Rev      int
	Kind     string
	Body     string
	LastSeen int64
}

// projectOf is the project a key belongs to, empty for the general memory.
func projectOf(key string) string {
	project, _, found := strings.Cut(key, "/")
	if !found {
		return ""
	}
	return project
}

// today is the day a fact written now gets, in days since the epoch.
func today() int64 {
	return time.Now().Unix() / 86400
}

// Render is the memory the prompt gets: every general fact and the newest of
// the project, in jimmy's own shape. When the project has more than fits, the
// keys of the rest ride along at the end so the model knows they exist.
func Render(general, project []Fact) string {
	entries := slices.Clone(general)
	slices.SortFunc(entries, func(a, b Fact) int { return cmp.Compare(a.Key, b.Key) })

	mine := slices.Clone(project)
	slices.SortFunc(mine, func(a, b Fact) int {
		if order := cmp.Compare(b.Date, a.Date); order != 0 {
			return order
		}
		return cmp.Compare(a.Key, b.Key)
	})
	fits := min(ProjectEntries, len(mine))
	outside := []string{}
	for _, fact := range mine[fits:] {
		outside = append(outside, fact.Key)
	}
	entries = append(entries, mine[:fits]...)

	if len(entries) == 0 {
		return ""
	}
	text := textOf(entries)
	if len(outside) == 0 {
		return text
	}
	return fmt.Sprintf("%s\n\n[afuera del prompt: %s]", text, strings.Join(outside, ", "))
}

// List is every fact the store holds, the general memory first and then a
// heading per project, in the shape `jimmy memo list` had. The headings are the
// scopes instead of the file paths, and inside a scope the order is the key.
func List(facts []Fact) string {
	ordered := slices.Clone(facts)
	slices.SortFunc(ordered, func(a, b Fact) int {
		if order := cmp.Compare(a.Project, b.Project); order != 0 {
			return order
		}
		return cmp.Compare(a.Key, b.Key)
	})
	lines := []string{}
	scope := ""
	for index, fact := range ordered {
		if index == 0 || fact.Project != scope {
			scope = fact.Project
			lines = append(lines, heading(scope))
		}
		lines = append(lines, fmt.Sprintf("  %s · %s · %s", fact.Key, fact.Kind, date(fact.Date)))
	}
	return strings.Join(lines, "\n")
}

// Show is one fact as the model reads it.
func Show(fact Fact) string {
	return textOf([]Fact{fact})
}

// ShowRevision is the fact as it was, marked so nobody takes it for the one
// that lives now.
func ShowRevision(revision Revision) string {
	return fmt.Sprintf("## %s · %s · %s (nivel 2)\n%s",
		revision.Key, revision.Kind, date(revision.LastSeen), strings.TrimSpace(revision.Body))
}

// Validate checks a key, a kind and a body before they reach the store.
func Validate(key, kind, body string) error {
	if !validKey(key) {
		return fmt.Errorf("clave inválida: %s (minúsculas, números, `-` y `familia/tema`)", key)
	}
	if !validKind(kind) {
		return fmt.Errorf("tipo desconocido: %s (los que hay: %s)", kind, strings.Join(Kinds, ", "))
	}
	if body == "" {
		return fmt.Errorf("el hecho está vacío")
	}
	return nil
}

func validKey(key string) bool {
	if key == "" || strings.HasPrefix(key, "/") || strings.HasSuffix(key, "/") || strings.Contains(key, "//") {
		return false
	}
	for _, c := range key {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '/', c == '.':
		default:
			return false
		}
	}
	return true
}

func validKind(kind string) bool {
	return slices.Contains(Kinds, kind)
}

func heading(project string) string {
	if project == "" {
		return "general"
	}
	return project
}

func textOf(facts []Fact) string {
	lines := make([]string, 0, len(facts))
	for _, fact := range facts {
		lines = append(lines, fmt.Sprintf("## %s · %s · %s\n\n%s",
			fact.Key, fact.Kind, date(fact.Date), strings.TrimSpace(fact.Body)))
	}
	return strings.Join(lines, "\n\n")
}

func date(days int64) string {
	return time.Unix(days*86400, 0).UTC().Format("2006-01-02")
}
