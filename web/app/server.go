package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/3-lines-studio/goddard/auth"
	"github.com/3-lines-studio/goddard/axe"
	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/heimdall"
	"github.com/3-lines-studio/goddard/memo"
	"github.com/3-lines-studio/goddard/org"
	"github.com/3-lines-studio/goddard/prompt"
	"github.com/3-lines-studio/goddard/schedule"
	"github.com/3-lines-studio/goddard/skill"
	"github.com/3-lines-studio/goddard/workspace"
)

// Serve opens the database and starts answering. The schema is not its
// business: what is missing is applied by hand with cmd/migrate, so an app that
// starts never changes the database it reads.
func Serve(ctx context.Context, handler http.Handler) error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("goddard: DATABASE_URL is not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	built, err := build(db)
	if err != nil {
		return err
	}
	Start(built)
	go built.Agenda.Serve(ctx, func(err error) { log.Printf("goddard: agenda: %v", err) })
	mux := http.NewServeMux()
	mux.Handle("GET /api/health", health(db))
	mux.Handle("/", handler)
	server := &http.Server{Addr: addr(), Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return err
	}
	err = <-done
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// offset is how far the local hour of the app is from UTC, in hours: the
// agenda needs it to know when a task is due.
func offset() int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(os.Getenv("GODDARD_TZ_OFFSET")), 10, 64)
	if err != nil {
		return 0
	}
	return value
}

func addr() string {
	if addr := os.Getenv("BIFROST_ADDR"); addr != "" {
		return addr
	}
	return ":8080"
}

// Volumes is where the projects live: a directory per owner inside it, and the
// project by its slug. It is the mount point of the volume in the sandbox —
// the machine the tools run in — and not a directory of this container, so
// goddard never reads it: what it does with it is hand it to the machine.
const Volumes = "/volumes"

// build opens every part of goddard over the same database and the same model
// of it: the stores, the agent, and who the app is being for.
func build(db *sql.DB) (*Service, error) {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		return nil, errors.New("goddard: OPENAI_API_KEY is not set")
	}
	master, err := masterKey()
	if err != nil {
		return nil, err
	}
	built := New(db, axe.NewOpenAI(env("GODDARD_BASE", "https://api.deepseek.com"), key), Volumes, master)
	built.Offset = offset()
	built.Mail = newMailer()
	built.Allowed = emails(os.Getenv("GODDARD_ALLOWED_EMAILS"))
	built.Model = env("GODDARD_MODEL", "deepseek-flash")
	built.Assistant = env("GODDARD_ASSISTANT", "Jimmy")
	built.Language = env("GODDARD_LANGUAGE", prompt.DefaultLanguage)
	built.Spec = env("GODDARD_PROMPT", prompt.Default)
	built.Agenda = schedule.NewService(built.Schedule, built.runTask, built.Offset)
	return built, nil
}

// New is the app over a database and a provider: the stores, the hub, the way
// to stop a turn, the volumes the projects live in and the way into the
// sandbox. `build` is this with everything else read from the environment,
// which is what a test of the app needs to skip.
func New(db *sql.DB, provider axe.Provider, volumes string, master heimdall.Key) *Service {
	built := &Service{
		DB:         db,
		Chat:       chat.NewStore(db),
		Auth:       auth.NewStore(db),
		Memo:       memo.NewPgStore(db),
		Skill:      skill.NewPgStore(db),
		Schedule:   schedule.NewPgStore(db),
		Orgs:       org.NewPgStore(db),
		Heimdall:   heimdall.NewStore(db, master),
		Workspaces: workspace.NewPgStore(db),
		Dialer:     SSH{},
		Provider:   provider,
		Hub:        newHub(),
		Stops:      newStops(),
		Volumes:    volumes,
	}
	built.Agenda = schedule.NewService(built.Schedule, built.runTask, 0)
	return built
}

// masterKey is what seals the secrets in heimdall: an owner's, under its own
// projects and environments. It is 32 bytes in hex, the same in every instance
// of the same goddard, and there is no way back: change it and nothing opens
// again.
func masterKey() (heimdall.Key, error) {
	text := os.Getenv("HEIMDALL_MASTER_KEY")
	if text == "" {
		return heimdall.Key{}, errors.New("goddard: HEIMDALL_MASTER_KEY is not set")
	}
	key, err := heimdall.KeyFromHex(text)
	if err != nil {
		return heimdall.Key{}, fmt.Errorf("goddard: HEIMDALL_MASTER_KEY: %w", err)
	}
	return key, nil
}

// emails is the comma separated list of who may ask for a link. Empty means
// anybody, which is a goddard of one with no list to keep.
func emails(list string) []string {
	out := []string{}
	for _, one := range strings.Split(list, ",") {
		if trimmed := strings.TrimSpace(one); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func health(db *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok\n"))
	})
}
