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

// visible is the memory a scope sees: the general facts of the person asking,
// and the ones of the project in hand.
const visible = `((owner_kind = $1 AND owner_id = $2 AND project = '') OR (owner_kind = $3 AND owner_id = $4 AND project = $5))`

// Add writes a fact, creating it or replacing the one under that key, and
// records the revision so the day it changed is not lost. A fact written again
// the same day, saying the same thing, changes nothing: rewriting a fact is
// not news.
func (s *PgStore) Add(ctx context.Context, scope Scope, key, kind, body string) (Outcome, error) {
	key = strings.TrimSpace(key)
	kind = strings.TrimSpace(kind)
	body = strings.TrimSpace(body)
	if err := Validate(key, kind, body); err != nil {
		return "", err
	}
	owner, project := scope.whereOf(key)
	day := today()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("no pude escribir el hecho %q: %w", key, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, owner.Kind+"/"+owner.ID+"/"+project+"/"+key); err != nil {
		return "", fmt.Errorf("no pude escribir el hecho %q: %w", key, err)
	}

	var previousKind, previousBody string
	var previousDay int64
	err = tx.QueryRowContext(ctx,
		`SELECT kind, body, fact_date FROM memo.facts WHERE owner_kind = $1 AND owner_id = $2 AND project = $3 AND key = $4`,
		owner.Kind, owner.ID, project, key).Scan(&previousKind, &previousBody, &previousDay)

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
		`INSERT INTO memo.facts (owner_kind, owner_id, project, key, kind, body, fact_date)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (owner_kind, owner_id, project, key) DO UPDATE SET
		     kind = EXCLUDED.kind,
		     body = EXCLUDED.body,
		     fact_date = EXCLUDED.fact_date`,
		owner.Kind, owner.ID, project, key, kind, body, day); err != nil {
		return "", fmt.Errorf("no pude guardar el hecho %q: %w", key, err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO memo.revisions (owner_kind, owner_id, project, key, rev, kind, body, last_seen)
		 SELECT $1, $2, $3, $4, COALESCE(MAX(rev), 0) + 1, $5, $6, $7
		 FROM memo.revisions WHERE owner_kind = $1 AND owner_id = $2 AND project = $3 AND key = $4`,
		owner.Kind, owner.ID, project, key, kind, body, day); err != nil {
		return "", fmt.Errorf("no pude guardar el hecho %q: %w", key, err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("no pude guardar el hecho %q: %w", key, err)
	}
	return outcome, nil
}

// Show is one fact as the model reads it, or the last revision there was when
// the fact is gone: it was dropped by hand and the store kept the record.
func (s *PgStore) Show(ctx context.Context, scope Scope, key string) (string, bool, error) {
	key = strings.TrimSpace(key)
	owner, project := scope.whereOf(key)
	facts, err := s.query(ctx,
		`SELECT `+factColumns+` FROM memo.facts WHERE owner_kind = $1 AND owner_id = $2 AND project = $3 AND key = $4`,
		owner.Kind, owner.ID, project, key)
	if err != nil {
		return "", false, err
	}
	if len(facts) > 0 {
		return Show(facts[0]), true, nil
	}

	var revision Revision
	err = s.db.QueryRowContext(ctx,
		`SELECT project, key, rev, kind, body, last_seen FROM memo.revisions
		 WHERE owner_kind = $1 AND owner_id = $2 AND project = $3 AND key = $4 ORDER BY rev DESC LIMIT 1`,
		owner.Kind, owner.ID, project, key).
		Scan(&revision.Project, &revision.Key, &revision.Rev, &revision.Kind, &revision.Body, &revision.LastSeen)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("no pude leer el hecho %q: %w", key, err)
	}
	return ShowRevision(revision), true, nil
}

// List is every fact this scope can see: the general ones of the person asking
// and the ones of the project in hand.
func (s *PgStore) List(ctx context.Context, scope Scope) (string, error) {
	facts, err := s.query(ctx,
		`SELECT `+factColumns+` FROM memo.facts WHERE `+visible,
		scope.User.Kind, scope.User.ID, scope.Project.Kind, scope.Project.ID, scope.Slug)
	if err != nil {
		return "", err
	}
	return List(facts), nil
}

// Facts is the memory of a project as data: the general facts and the ones of
// that project, in order and with their date. It is what the prompt renders
// and what a page can show.
func (s *PgStore) Facts(ctx context.Context, scope Scope) ([]Fact, error) {
	return s.query(ctx,
		`SELECT `+factColumns+` FROM memo.facts WHERE `+visible+` ORDER BY project, key`,
		scope.User.Kind, scope.User.ID, scope.Project.Kind, scope.Project.ID, scope.Slug)
}

// Render is the memory the prompt gets for that project: the general facts and
// the newest of the project.
func (s *PgStore) Render(ctx context.Context, scope Scope) (string, error) {
	facts, err := s.query(ctx,
		`SELECT `+factColumns+` FROM memo.facts WHERE `+visible,
		scope.User.Kind, scope.User.ID, scope.Project.Kind, scope.Project.ID, scope.Slug)
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
