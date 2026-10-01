package memo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// PgStore keeps the facts in the memo schema of the shared database (see
// migrations/), over the same pool as the rest of goddard.
type PgStore struct {
	db *sql.DB
}

func NewPgStore(db *sql.DB) *PgStore {
	return &PgStore{db: db}
}

// Outcome is what an Add did to the store.
type Outcome string

const (
	Created    Outcome = "nuevo"
	Updated    Outcome = "actualizado"
	Reasserted Outcome = "reafirmado"
	Unchanged  Outcome = "sin cambios"
)

const factColumns = `project, key, kind, body, fact_date`

// Add writes a fact, creating it or replacing the one under that key, and
// records the revision so the day it changed is not lost. A fact written again
// the same day, saying the same thing, changes nothing: rewriting a fact is
// not news.
func (s *PgStore) Add(ctx context.Context, key, kind, body string) (Outcome, error) {
	key = strings.TrimSpace(key)
	kind = strings.TrimSpace(kind)
	body = strings.TrimSpace(body)
	if err := Validate(key, kind, body); err != nil {
		return "", err
	}
	project := projectOf(key)
	day := today()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("no pude escribir el hecho %q: %w", key, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, project+"/"+key); err != nil {
		return "", fmt.Errorf("no pude escribir el hecho %q: %w", key, err)
	}

	var previousKind, previousBody string
	var previousDay int64
	err = tx.QueryRowContext(ctx,
		`SELECT kind, body, fact_date FROM memo.facts WHERE project = $1 AND key = $2`,
		project, key).Scan(&previousKind, &previousBody, &previousDay)

	outcome := Created
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return "", fmt.Errorf("no pude leer el hecho %q: %w", key, err)
	case previousKind != kind || previousBody != body:
		outcome = Updated
	case previousDay != day:
		outcome = Reasserted
	default:
		return Unchanged, nil
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO memo.facts (project, key, kind, body, fact_date)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (project, key) DO UPDATE SET
		     kind = EXCLUDED.kind,
		     body = EXCLUDED.body,
		     fact_date = EXCLUDED.fact_date`,
		project, key, kind, body, day); err != nil {
		return "", fmt.Errorf("no pude guardar el hecho %q: %w", key, err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO memo.revisions (project, key, rev, kind, body, last_seen)
		 SELECT $1, $2, COALESCE(MAX(rev), 0) + 1, $3, $4, $5
		 FROM memo.revisions WHERE project = $1 AND key = $2`,
		project, key, kind, body, day); err != nil {
		return "", fmt.Errorf("no pude guardar el hecho %q: %w", key, err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("no pude guardar el hecho %q: %w", key, err)
	}
	return outcome, nil
}

// Show is one fact as the model reads it, or the last revision there was when
// the fact is gone: it was dropped by hand and the store kept the record.
func (s *PgStore) Show(ctx context.Context, key string) (string, bool, error) {
	key = strings.TrimSpace(key)
	facts, err := s.query(ctx,
		`SELECT `+factColumns+` FROM memo.facts WHERE key = $1`, key)
	if err != nil {
		return "", false, err
	}
	if len(facts) > 0 {
		return Show(facts[0]), true, nil
	}

	var revision Revision
	err = s.db.QueryRowContext(ctx,
		`SELECT project, key, rev, kind, body, last_seen FROM memo.revisions
		 WHERE key = $1 ORDER BY rev DESC LIMIT 1`, key).
		Scan(&revision.Project, &revision.Key, &revision.Rev, &revision.Kind, &revision.Body, &revision.LastSeen)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("no pude leer el hecho %q: %w", key, err)
	}
	return ShowRevision(revision), true, nil
}

// List is every fact the store holds.
func (s *PgStore) List(ctx context.Context) (string, error) {
	facts, err := s.query(ctx, `SELECT `+factColumns+` FROM memo.facts`)
	if err != nil {
		return "", err
	}
	return List(facts), nil
}

// Render is the memory the prompt gets for that project: the general facts and
// the newest of the project.
func (s *PgStore) Render(ctx context.Context, project string) (string, error) {
	facts, err := s.query(ctx,
		`SELECT `+factColumns+` FROM memo.facts WHERE project = '' OR project = $1`, project)
	if err != nil {
		return "", err
	}
	general := []Fact{}
	mine := []Fact{}
	for _, fact := range facts {
		if fact.Project == "" {
			general = append(general, fact)
			continue
		}
		mine = append(mine, fact)
	}
	return Render(general, mine), nil
}

func (s *PgStore) query(ctx context.Context, statement string, args ...any) ([]Fact, error) {
	rows, err := s.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("no pude leer la memoria: %w", err)
	}
	defer rows.Close()
	facts := []Fact{}
	for rows.Next() {
		var fact Fact
		if err := rows.Scan(&fact.Project, &fact.Key, &fact.Kind, &fact.Body, &fact.Date); err != nil {
			return nil, fmt.Errorf("no pude leer un hecho: %w", err)
		}
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("no pude leer la memoria: %w", err)
	}
	return facts, nil
}
