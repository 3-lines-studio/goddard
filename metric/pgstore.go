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

// Summary adds up the turns of a window, day by day and in total. The offset is
// the hours a day is shifted by — the same one the agenda uses — so a day is
// the day of whoever is reading. What comes back are numbers: no owner, no
// project and no conversation, which is what somebody who is not the owner may
// see.
func (s *PgStore) Summary(ctx context.Context, from, until, offset int64) (Summary, error) {
	out := Summary{From: from, Until: until, Days: []Day{}}
	rows, err := s.db.QueryContext(ctx, daysQuery, from, until, offset*3600)
	if err != nil {
		return Summary{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var day Day
		err := rows.Scan(&day.Date, &day.Model, &day.Totals.Turns, &day.Totals.Input,
			&day.Totals.Output, &day.Totals.CachedInput, &day.Totals.Ms,
			&day.Totals.Failed, &day.Totals.Cancelled)
		if err != nil {
			return Summary{}, err
		}
		out.Days = append(out.Days, day)
	}
	if err := rows.Err(); err != nil {
		return Summary{}, err
	}
	var total Totals
	err = s.db.QueryRowContext(ctx, totalQuery, from, until).Scan(&total.Turns,
		&total.Input, &total.Output, &total.CachedInput, &total.Ms, &total.Failed,
		&total.Cancelled)
	if err != nil {
		return Summary{}, err
	}
	out.Total = total
	return out, nil
}

const daysQuery = `
	SELECT (to_timestamp(created_at + $3))::date::text, model,
	       count(*), coalesce(sum(input), 0), coalesce(sum(output), 0),
	       coalesce(sum(cached_input), 0), coalesce(sum(ms), 0),
	       count(*) FILTER (WHERE outcome = 'failed'),
	       count(*) FILTER (WHERE outcome = 'cancelled')
	  FROM metric.turns
	 WHERE deleted_at IS NULL AND created_at >= $1 AND created_at < $2
	 GROUP BY 1, model
	 ORDER BY 1 DESC, model`

const totalQuery = `
	SELECT count(*), coalesce(sum(input), 0), coalesce(sum(output), 0),
	       coalesce(sum(cached_input), 0), coalesce(sum(ms), 0),
	       count(*) FILTER (WHERE outcome = 'failed'),
	       count(*) FILTER (WHERE outcome = 'cancelled')
	  FROM metric.turns
	 WHERE deleted_at IS NULL AND created_at >= $1 AND created_at < $2`
