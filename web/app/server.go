package app

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/3-lines-studio/goddard/auth"
	"github.com/3-lines-studio/goddard/axe"
	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/memo"
	"github.com/3-lines-studio/goddard/migrations"
	"github.com/3-lines-studio/goddard/prompt"
	"github.com/3-lines-studio/goddard/schedule"
	"github.com/3-lines-studio/goddard/skill"
)

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
	if _, err := migrations.Apply(ctx, db); err != nil {
		return err
	}
	built, err := build(db)
	if err != nil {
		return err
	}
	running(built)
	agenda := schedule.NewService(built.Schedule, built.runTask, built.Offset)
	go agenda.Serve(ctx, func(err error) { log.Printf("goddard: agenda: %v", err) })
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

// build opens every part of goddard over the same database and the same model
// of it: the stores, the agent, and who the app is being for.
func build(db *sql.DB) (*Service, error) {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		return nil, errors.New("goddard: OPENAI_API_KEY is not set")
	}
	workspace := env("GODDARD_WORKSPACE", ".")
	if absolute, err := filepath.Abs(workspace); err == nil {
		workspace = absolute
	}
	return &Service{
		DB:        db,
		Chat:      chat.NewStore(db),
		Auth:      auth.NewStore(db),
		Memo:      memo.NewPgStore(db),
		Skill:     skill.NewPgStore(db),
		Schedule:  schedule.NewPgStore(db),
		Provider:  axe.NewOpenAI(env("GODDARD_BASE", "https://api.deepseek.com"), key),
		Offset:    offset(),
		Hub:       newHub(),
		Mail:      newMailer(),
		Allowed:   emails(os.Getenv("GODDARD_ALLOWED_EMAILS")),
		Model:     env("GODDARD_MODEL", "deepseek-flash"),
		Workspace: workspace,
		Viewer:    skill.Viewer{Org: env("GODDARD_ORG", "3-lines-studio"), User: env("GODDARD_USER", "berti")},
		User:      env("GODDARD_USER_NAME", "Don Berti"),
		Assistant: env("GODDARD_ASSISTANT", "Jimmy"),
		Language:  env("GODDARD_LANGUAGE", prompt.DefaultLanguage),
		Spec:      env("GODDARD_PROMPT", prompt.Default),
	}, nil
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
