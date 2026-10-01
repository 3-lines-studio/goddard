package app

import (
	"database/sql"
	"os"
	"sync"
	"time"

	"github.com/3-lines-studio/goddard/auth"
	"github.com/3-lines-studio/goddard/axe"
	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/memo"
	"github.com/3-lines-studio/goddard/schedule"
	"github.com/3-lines-studio/goddard/skill"
)

// Service is what the routes need: the stores of every part of goddard and the
// agent that answers. The routes are packages of their own in the same tree,
// with functions per method and no way to hand them anything, so this is the
// door they come through.
type Service struct {
	DB        *sql.DB
	Chat      *chat.Store
	Auth      *auth.Store
	Memo      *memo.PgStore
	Skill     *skill.PgStore
	Schedule  *schedule.PgStore
	Provider  axe.Provider
	Model     string
	Workspace string
	Viewer    skill.Viewer
	User      string
	Assistant string
	Language  string
	Spec      string
}

var (
	serviceMu sync.RWMutex
	service   *Service
)

// Current is the service that is running, or nil before it started.
func Current() *Service {
	serviceMu.RLock()
	defer serviceMu.RUnlock()
	return service
}

func running(built *Service) {
	serviceMu.Lock()
	service = built
	serviceMu.Unlock()
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// Running says whether a turn is going on in that conversation right now. The
// claim is what knows, and it lives in the database, so any instance answers
// the same.
func (s *Service) Running(conversation chat.Conversation) bool {
	return conversation.ClaimedUntil > time.Now().Unix()
}
