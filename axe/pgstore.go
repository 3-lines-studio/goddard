package axe

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// PgStore keeps a conversation in Postgres, in the axe schema of the shared
// database (see migrations/), over the same pool as the rest of goddard.
//
// A session is a row of axe.sessions and each entry is a row of axe.entries,
// so appending is an INSERT and the order is the seq. The live session is the
// one with archived_at NULL, at most one per scope, and archiving it is an
// UPDATE. A scope is whatever the app says it is — a chat, a project, a
// user — and two stores over the same database and scope see the same history.
//
// The id of a session is the millisecond it was created, with a numeric suffix
// when another session already took that millisecond.
type PgStore struct {
	db    *sql.DB
	scope string
}

func NewPgStore(db *sql.DB, scope string) *PgStore {
	return &PgStore{db: db, scope: scope}
}

type liveRow struct {
	id    string
	title string
	turns int
}

func (s *PgStore) Live(ctx context.Context) ([]Entry, error) {
	row, err := liveIn(ctx, s.db, s.scope)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return []Entry{}, nil
	}
	return entriesOf(ctx, s.db, row.id)
}

// Save replaces the live session with these entries, which is what a resumed
// conversation does with the transcript it loaded.
func (s *PgStore) Save(ctx context.Context, entries []Entry) error {
	if len(entries) == 0 {
		return nil
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		row, err := ensureLive(ctx, tx, s.scope)
		if err != nil {
			return err
		}
		if err := replace(ctx, tx, row.id, entries); err != nil {
			return err
		}
		title, turns := foldMeta(untitled, 0, entries)
		return touch(ctx, tx, row.id, title, turns)
	})
}

func (s *PgStore) Append(ctx context.Context, entries []Entry) error {
	if len(entries) == 0 {
		return nil
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		row, err := ensureLive(ctx, tx, s.scope)
		if err != nil {
			return err
		}
		next, err := nextSeq(ctx, tx, row.id)
		if err != nil {
			return err
		}
		if err := insert(ctx, tx, row.id, next, entries); err != nil {
			return err
		}
		title, turns := foldMeta(row.title, row.turns, entries)
		return touch(ctx, tx, row.id, title, turns)
	})
}

// Archive moves the live session into the archive and returns its id. With a
// resume id pending, the live session continues that one instead of forking:
// its entries replace the archived ones, exactly as rewriting the file did.
func (s *PgStore) Archive(ctx context.Context) (string, bool, error) {
	archived := ""
	found := false
	err := s.write(ctx, func(tx *sql.Tx) error {
		row, err := liveIn(ctx, tx, s.scope)
		if err != nil || row == nil {
			return err
		}
		resume, pending, err := resumeIn(ctx, tx, s.scope)
		if err != nil {
			return err
		}
		if pending {
			if err := moveInto(ctx, tx, s.scope, row, resume); err != nil {
				return err
			}
			if err := clearResume(ctx, tx, s.scope); err != nil {
				return err
			}
			archived, found = resume, true
			return nil
		}
		if err := markArchived(ctx, tx, row.id, row.title, row.turns); err != nil {
			return err
		}
		archived, found = row.id, true
		return nil
	})
	return archived, found, err
}

// ContinueArchived writes these entries back into the archived session id and
// clears the live session, so a conversation that was reopened keeps its id.
func (s *PgStore) ContinueArchived(ctx context.Context, id string, entries []Entry) (bool, error) {
	if !validID(id) || len(entries) == 0 {
		return false, nil
	}
	err := s.write(ctx, func(tx *sql.Tx) error {
		title, turns := foldMeta(untitled, 0, entries)
		if err := putArchived(ctx, tx, s.scope, id, title, turns); err != nil {
			return err
		}
		if err := replace(ctx, tx, id, entries); err != nil {
			return err
		}
		if err := dropLive(ctx, tx, s.scope, id); err != nil {
			return err
		}
		return clearResume(ctx, tx, s.scope)
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *PgStore) ContinueArchivedLive(ctx context.Context, id string) (bool, error) {
	if !validID(id) {
		return false, nil
	}
	found := false
	err := s.write(ctx, func(tx *sql.Tx) error {
		row, err := liveIn(ctx, tx, s.scope)
		if err != nil || row == nil {
			return err
		}
		if err := moveInto(ctx, tx, s.scope, row, id); err != nil {
			return err
		}
		if err := clearResume(ctx, tx, s.scope); err != nil {
			return err
		}
		found = true
		return nil
	})
	return found, err
}

func (s *PgStore) Discard(ctx context.Context) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM axe.sessions WHERE scope = $1 AND archived_at IS NULL", s.scope); err != nil {
			return err
		}
		return clearResume(ctx, tx, s.scope)
	})
}

func (s *PgStore) List(ctx context.Context) ([]SessionMeta, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, title, updated_at, turns FROM axe.sessions "+
			"WHERE scope = $1 AND archived_at IS NOT NULL ORDER BY updated_at DESC, seq DESC", s.scope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sessions := []SessionMeta{}
	for rows.Next() {
		var session SessionMeta
		if err := rows.Scan(&session.ID, &session.Title, &session.Updated, &session.Turns); err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func (s *PgStore) Load(ctx context.Context, id string) ([]Entry, bool, error) {
	if !validID(id) {
		return nil, false, nil
	}
	var archived bool
	err := s.db.QueryRowContext(ctx,
		"SELECT archived_at IS NOT NULL FROM axe.sessions WHERE scope = $1 AND id = $2", s.scope, id).
		Scan(&archived)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !archived {
		return nil, false, nil
	}
	entries, err := entriesOf(ctx, s.db, id)
	if err != nil {
		return nil, false, err
	}
	return entries, true, nil
}

func (s *PgStore) ResumeID(ctx context.Context) (string, bool, error) {
	id, found, err := resumeIn(ctx, s.db, s.scope)
	return id, found, err
}

func (s *PgStore) SetResumeID(ctx context.Context, id string) error {
	if !validID(id) {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO axe.resume (scope, session_id) VALUES ($1, $2) "+
			"ON CONFLICT (scope) DO UPDATE SET session_id = excluded.session_id", s.scope, id)
	return err
}

func (s *PgStore) ClearResumeID(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM axe.resume WHERE scope = $1", s.scope)
	return err
}

// write runs one change in a transaction under an advisory lock of the scope:
// two stores appending to the same conversation at once end up one after the
// other instead of fighting over the seq and the live row.
func (s *PgStore) write(ctx context.Context, work func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", s.scope); err != nil {
		return err
	}
	if err := work(tx); err != nil {
		return err
	}
	return tx.Commit()
}

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func liveIn(ctx context.Context, db querier, scope string) (*liveRow, error) {
	var row liveRow
	err := db.QueryRowContext(ctx,
		"SELECT id, title, turns FROM axe.sessions WHERE scope = $1 AND archived_at IS NULL", scope).
		Scan(&row.id, &row.title, &row.turns)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func ensureLive(ctx context.Context, tx *sql.Tx, scope string) (*liveRow, error) {
	row, err := liveIn(ctx, tx, scope)
	if err != nil || row != nil {
		return row, err
	}
	base := NowMs()
	for suffix := 0; suffix < 1000; suffix++ {
		id := fmt.Sprintf("%d", base)
		if suffix > 0 {
			id = fmt.Sprintf("%d-%d", base, suffix)
		}
		result, err := tx.ExecContext(ctx,
			"INSERT INTO axe.sessions (id, scope, title, turns, updated_at) VALUES ($1, $2, $3, 0, $4) "+
				"ON CONFLICT (id) DO NOTHING", id, scope, untitled, base)
		if err != nil {
			return nil, err
		}
		created, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if created == 1 {
			return &liveRow{id: id, title: untitled}, nil
		}
	}
	return nil, fmt.Errorf("axe: no pude acuñar una sesión para el scope %q", scope)
}

func resumeIn(ctx context.Context, db querier, scope string) (string, bool, error) {
	var id string
	err := db.QueryRowContext(ctx, "SELECT session_id FROM axe.resume WHERE scope = $1", scope).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return id, true, nil
}

func clearResume(ctx context.Context, tx *sql.Tx, scope string) error {
	_, err := tx.ExecContext(ctx, "DELETE FROM axe.resume WHERE scope = $1", scope)
	return err
}

func entriesOf(ctx context.Context, db querier, id string) ([]Entry, error) {
	rows, err := db.QueryContext(ctx, "SELECT entry FROM axe.entries WHERE session_id = $1 ORDER BY seq", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []Entry{}
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var entry Entry
		if err := json.Unmarshal(data, &entry); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func nextSeq(ctx context.Context, tx *sql.Tx, id string) (int64, error) {
	var last sql.NullInt64
	err := tx.QueryRowContext(ctx,
		"SELECT max(seq) FROM axe.entries WHERE session_id = $1", id).Scan(&last)
	if err != nil {
		return 0, err
	}
	return last.Int64 + 1, nil
}

func insert(ctx context.Context, tx *sql.Tx, id string, next int64, entries []Entry) error {
	for _, entry := range entries {
		data, err := encodeJSON(entry)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO axe.entries (session_id, seq, entry) VALUES ($1, $2, $3::jsonb)",
			id, next, string(data)); err != nil {
			return err
		}
		next++
	}
	return nil
}

func replace(ctx context.Context, tx *sql.Tx, id string, entries []Entry) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM axe.entries WHERE session_id = $1", id); err != nil {
		return err
	}
	return insert(ctx, tx, id, 1, entries)
}

// moveInto makes the live session the archived one `id`: the entries replace
// whatever that session had, and the live row goes away.
func moveInto(ctx context.Context, tx *sql.Tx, scope string, live *liveRow, id string) error {
	if id == live.id {
		return markArchived(ctx, tx, id, live.title, live.turns)
	}
	if err := putArchived(ctx, tx, scope, id, live.title, live.turns); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM axe.entries WHERE session_id = $1", id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO axe.entries (session_id, seq, entry) SELECT $1, seq, entry FROM axe.entries WHERE session_id = $2",
		id, live.id); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "DELETE FROM axe.sessions WHERE id = $1", live.id)
	return err
}

func putArchived(ctx context.Context, tx *sql.Tx, scope, id, title string, turns int) error {
	var owner string
	err := tx.QueryRowContext(ctx, "SELECT scope FROM axe.sessions WHERE id = $1", id).Scan(&owner)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO axe.sessions (id, scope, title, turns, updated_at, archived_at) VALUES ($1, $2, $3, $4, $5, $5)",
			id, scope, title, turns, NowMs())
		return err
	}
	if owner != scope {
		return fmt.Errorf("axe: la sesión %q es de otro scope", id)
	}
	return markArchived(ctx, tx, id, title, turns)
}

func dropLive(ctx context.Context, tx *sql.Tx, scope, keep string) error {
	_, err := tx.ExecContext(ctx,
		"DELETE FROM axe.sessions WHERE scope = $1 AND archived_at IS NULL AND id <> $2", scope, keep)
	return err
}

func touch(ctx context.Context, tx *sql.Tx, id, title string, turns int) error {
	_, err := tx.ExecContext(ctx,
		"UPDATE axe.sessions SET title = $2, turns = $3, updated_at = $4 WHERE id = $1",
		id, title, turns, NowMs())
	return err
}

func markArchived(ctx context.Context, tx *sql.Tx, id, title string, turns int) error {
	_, err := tx.ExecContext(ctx,
		"UPDATE axe.sessions SET title = $2, turns = $3, updated_at = $4, archived_at = $4 WHERE id = $1",
		id, title, turns, NowMs())
	return err
}

// foldMeta applies entries to a session's title and turn count. A user message
// adds a turn; a compaction restarts the count at the checkpoint plus the user
// messages it retains. The title is the first words of the first user message,
// or of the last summary once the conversation compacted.
func foldMeta(title string, turns int, entries []Entry) (string, int) {
	for _, entry := range entries {
		turns = nextTurns(turns, entry)
		switch entry.Type {
		case EntryMessage:
			if entry.Message.Role == "user" && title == untitled && entry.Message.Content != "" {
				title = firstWords(entry.Message.Content, 8)
			}
		case EntryCompaction:
			title = firstWords(CompactionPrefix+entry.Summary+CompactionSuffix, 8)
		}
	}
	return title, turns
}
