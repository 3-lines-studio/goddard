package app

import (
	"database/sql"
	"net/http"
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
	Offset    int64
	Agenda    *schedule.Service
	Stops     *stops
	Hub       *hub
	Mail      *mailer
	Allowed   []string
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

// Start hands the routes the app they answer with. Serve calls it, and a test
// of one route calls it too, which is what makes a route reachable without the
// binary around it.
func Start(built *Service) {
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

// Session is the user behind the request, and it answers 401 when there is
// none. Every route that touches a conversation goes through here.
func Session(service *Service, w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	user, ok := service.Who(r.Context(), r)
	if !ok {
		http.Error(w, "no hay sesión", http.StatusUnauthorized)
	}
	return user, ok
}

// Live is what a conversation is saying while a turn is still running, and the
// way to stop listening.
func (s *Service) Live(conversation string) (chan []byte, func()) {
	return s.Hub.listen(conversation)
}
