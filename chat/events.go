package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// Event is one line of a conversation's log. The body is the event as the
// client reads it — the dialect is the app's, not this store's — and the seq
// is what the stream uses to say what it already saw.
type Event struct {
	Seq  int64
	At   int64
	Body json.RawMessage
}

// Append writes one line and returns it with the seq it got. Two writers on
// the same thread take turns instead of fighting for the number, and the
// conversation is marked as touched in the same transaction.
func (s *Store) Append(ctx context.Context, conversationID string, body []byte) (Event, error) {
	event := Event{Body: body}
	err := s.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", conversationID); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx,
			"INSERT INTO chat.events (conversation_id, seq, at, event) VALUES "+
				"($1, (SELECT COALESCE(MAX(seq), 0) + 1 FROM chat.events WHERE conversation_id = $1), goddard.now(), $2::jsonb) "+
				"RETURNING seq, at",
			conversationID, string(body)).Scan(&event.Seq, &event.At); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			"UPDATE chat.conversations SET updated_at = goddard.now() WHERE id = $1", conversationID)
		return err
	})
	if err != nil {
		return Event{}, err
	}
	return event, nil
}

// Events is the log from a point on, oldest first. Without a since it is the
// whole thread, which is what a client that just opened the conversation asks
// for.
func (s *Store) Events(ctx context.Context, conversationID string, since int64) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT seq, at, event FROM chat.events "+
			"WHERE conversation_id = $1 AND seq > $2 AND deleted_at IS NULL ORDER BY seq",
		conversationID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []Event{}
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.Seq, &event.At, &event.Body); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) write(ctx context.Context, work func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := work(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// Claim takes the conversation for one turn. It says no when somebody else is
// already running one there: with several instances serving, the lease is the
// only way to know, and it expires so a dead instance does not hold the thread
// forever.
func (s *Store) Claim(ctx context.Context, id string, lease int64) (bool, error) {
	var claimed string
	err := s.db.QueryRowContext(ctx,
		"UPDATE chat.conversations SET claimed_until = goddard.now() + $2, updated_at = goddard.now() "+
			"WHERE id = $1 AND deleted_at IS NULL AND (claimed_until IS NULL OR claimed_until <= goddard.now()) "+
			"RETURNING id", id, lease).Scan(&claimed)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Release gives the conversation back when the turn is over, whether it
// answered or not.
func (s *Store) Release(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE chat.conversations SET claimed_until = NULL WHERE id = $1", id)
	return err
}
