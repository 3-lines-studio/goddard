// Package chat keeps the projects and the conversations of goddard: a project
// is a place where work lives, a conversation is a thread inside it, and the
// events of that thread live in the same schema so any instance can serve it.
package chat

import (
	"errors"
)

// ErrTaken is a project whose slug is already somebody else's.
var ErrTaken = errors.New("ese proyecto ya existe")

// Project is a place where work lives: its slug names the directory of the
// workspace, and it is the string the rest of goddard already knows — memo,
// schedule and heimdall keep it in their own tables.
type Project struct {
	ID        string `json:"id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	CreatedBy string `json:"created_by"`
}

// Conversation is one thread inside a project. Its id is the scope an axe
// session uses, so the history of the conversation is the history of that
// session and nobody has to keep the two in step.
type Conversation struct {
	ID           string `json:"id"`
	ProjectID    string `json:"project_id"`
	Title        string `json:"title"`
	Source       string `json:"source"`
	CreatedBy    string `json:"created_by"`
	ClaimedUntil int64  `json:"claimed_until"`
	UpdatedAt    int64  `json:"updated_at"`
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
