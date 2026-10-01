// Package chat keeps the projects and the conversations of goddard: a project
// is a place where work lives, a conversation is a thread inside it, and the
// events of that thread live in the same schema so any instance can serve it.
package chat

import (
	"errors"
	"strings"
	"unicode"
)

// ErrTaken is a project whose slug is already somebody else's.
var ErrTaken = errors.New("ese proyecto ya existe")

// Project is a place where work lives: its slug names the directory of the
// workspace, and it is the string the rest of goddard already knows — memo,
// schedule and heimdall keep it in their own tables.
type Project struct {
	ID        string
	Slug      string
	Name      string
	CreatedBy string
}

// Conversation is one thread inside a project. Its id is the scope an axe
// session uses, so the history of the conversation is the history of that
// session and nobody has to keep the two in step.
type Conversation struct {
	ID        string
	ProjectID string
	Title     string
	Source    string
	CreatedBy string
	UpdatedAt int64
}

// Sources are where a conversation comes from. A conversation that came from a
// transport is read from the web and not written there.
const (
	SourceWeb      = "web"
	SourceTelegram = "telegram"
	SourceSlack    = "slack"
	SourceSchedule = "schedule"
)

// NewTitle is what a conversation that just started is called until the first
// message names it, the same words jimmy used.
const NewTitle = "nueva conversación"

// Slug turns a name into what a directory and the other tables can hold:
// lowercase, no accents, no punctuation, words joined by single dashes.
func Slug(name string) string {
	var builder strings.Builder
	dashed := false
	for _, r := range name {
		switch {
		case r == '\'' || r == '’':
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if dashed && builder.Len() > 0 {
				builder.WriteByte('-')
			}
			dashed = false
			builder.WriteRune(plain(r))
		default:
			dashed = true
		}
	}
	return builder.String()
}

func plain(r rune) rune {
	r = unicode.ToLower(r)
	switch r {
	case 'á', 'à', 'ä', 'â', 'ã', 'å':
		return 'a'
	case 'é', 'è', 'ë', 'ê':
		return 'e'
	case 'í', 'ì', 'ï', 'î':
		return 'i'
	case 'ó', 'ò', 'ö', 'ô', 'õ':
		return 'o'
	case 'ú', 'ù', 'ü', 'û':
		return 'u'
	case 'ñ':
		return 'n'
	case 'ç':
		return 'c'
	}
	return r
}
