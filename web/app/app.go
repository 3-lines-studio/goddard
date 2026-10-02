package app

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/3-lines-studio/goddard/auth"
	"github.com/3-lines-studio/goddard/axe"
	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/heimdall"
	"github.com/3-lines-studio/goddard/memo"
	"github.com/3-lines-studio/goddard/metric"
	"github.com/3-lines-studio/goddard/org"
	"github.com/3-lines-studio/goddard/schedule"
	"github.com/3-lines-studio/goddard/skill"
	"github.com/3-lines-studio/goddard/workspace"
)

// Service is what the routes need: the stores of every part of goddard and the
// agent that answers. The routes are packages of their own in the same tree,
// with functions per method and no way to hand them anything, so this is the
// door they come through.
type Service struct {
	DB         *sql.DB
	Chat       *chat.Store
	Auth       *auth.Store
	Memo       *memo.PgStore
	Skill      *skill.PgStore
	Schedule   *schedule.PgStore
	Provider   axe.Provider
	Offset     int64
	Agenda     *schedule.Service
	Stops      *stops
	Hub        *hub
	Mail       *mailer
	Allowed    []string
	Admins     []string
	Model      string
	Volumes    string
	Orgs       *org.PgStore
	Heimdall   *heimdall.Store
	Workspaces *workspace.PgStore
	Metric     *metric.PgStore
	Dialer     Dialer
	Assistant  string
	Language   string
	Spec       string
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

// Admin is whether this user reads what only the house reads: the numbers of
// every owner. An empty list is nobody, so a goddard without admins has no
// admin view.
func (s *Service) Admin(user auth.User) bool {
	for _, one := range s.Admins {
		if strings.EqualFold(strings.TrimSpace(one), user.Email) {
			return true
		}
	}
	return false
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// Viewer is who a turn is for: the id the database minted, which is what the
// skills, the agenda and the memory keep, and the organizations that person is
// in. It comes from the session and not from the environment, because the
// environment is one for every user.
func (s *Service) Viewer(ctx context.Context, user auth.User) skill.Viewer {
	viewer := skill.Viewer{User: user.ID}
	orgs, err := s.Orgs.Orgs(ctx, user.ID)
	if err != nil {
		log.Printf("goddard: no pude leer las organizaciones de %s: %v", user.ID, err)
		return viewer
	}
	for _, one := range orgs {
		viewer.Orgs = append(viewer.Orgs, one.ID)
	}
	return viewer
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

// Yours is the conversation this user may read, or nothing: whoever created it,
// or anybody in the organization it belongs to. It is what every route that
// answers with the content of a thread asks for first.
func (s *Service) Yours(ctx context.Context, user auth.User, conversationID string) (chat.Conversation, bool, error) {
	conversation, ok, err := s.Chat.Conversation(ctx, conversationID)
	if err != nil || !ok {
		return chat.Conversation{}, false, err
	}
	if err := s.authorize(ctx, s.Viewer(ctx, user), conversation); err != nil {
		return chat.Conversation{}, false, nil
	}
	return conversation, true, nil
}

// Mine is the project this user may change, or nothing: their own, or the one
// of an organization they are in. A project of somebody else is invisible
// here too, which is why the routes answer 404 and not 403: the name of a
// project somebody else has is not something to confirm.
func (s *Service) Mine(ctx context.Context, user auth.User, projectID string) (chat.Project, bool, error) {
	project, ok, err := s.Chat.Project(ctx, projectID)
	if err != nil || !ok {
		return chat.Project{}, false, err
	}
	if project.Owner.Kind == chat.OwnerUser {
		return project, project.Owner.ID == user.ID, nil
	}
	_, in, err := s.Orgs.Role(ctx, project.Owner.ID, user.ID)
	if err != nil {
		return chat.Project{}, false, err
	}
	return project, in, nil
}

// Live is what a conversation is saying while a turn is still running, and the
// way to stop listening.
func (s *Service) Live(conversation string) (chan []byte, func()) {
	return s.Hub.listen(conversation)
}
