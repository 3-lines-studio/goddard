package metric

import (
	"context"
	"database/sql"
)

// PgStore keeps the turns of every owner in the `metric` schema of the shared
// database (see migrations/), over the same pool as the rest of goddard.
type PgStore struct {
	db *sql.DB
}

func NewPgStore(db *sql.DB) *PgStore {
	return &PgStore{db: db}
}

// Record writes one turn. It is called when the turn is over, and it never
// decides anything: a turn that cannot be noted is a line in the log, not a
// turn that did not happen.
func (s *PgStore) Record(ctx context.Context, turn Turn) error {
	if err := turn.Valid(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO metric.turns
		     (owner_kind, owner_id, project_id, conversation_id, user_id, source,
		      model, input, output, cached_input, ms, outcome)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		turn.OwnerKind, turn.OwnerID, turn.ProjectID, turn.ConversationID,
		turn.UserID, turn.Source, turn.Model, turn.Input, turn.Output,
		turn.CachedInput, turn.Ms, turn.Outcome)
	return err
}
