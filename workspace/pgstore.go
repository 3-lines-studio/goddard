package workspace

import (
	"context"
	"database/sql"
	"errors"
)

// PgStore keeps the config of every workspace in the `workspace` schema of the
// shared database (see migrations/), over the same pool as the rest of
// goddard.
type PgStore struct {
	db *sql.DB
}

func NewPgStore(db *sql.DB) *PgStore {
	return &PgStore{db: db}
}

// Get is the row of the workspace of an owner, or nothing when there is none:
// where the projects of an owner live is deployment config — the volume is
// mounted somewhere — and whoever calls this is the one that knows the root.
func (s *PgStore) Get(ctx context.Context, owner Owner) (Workspace, bool, error) {
	if err := checkOwner(owner); err != nil {
		return Workspace{}, false, err
	}
	found := Workspace{Owner: owner}
	err := s.db.QueryRowContext(ctx,
		`SELECT path FROM workspace.workspaces WHERE owner_kind = $1 AND owner_id = $2`,
		owner.Kind, owner.ID).Scan(&found.Path)
	if errors.Is(err, sql.ErrNoRows) {
		return Workspace{}, false, nil
	}
	if err != nil {
		return Workspace{}, false, err
	}
	return found, true, nil
}

// Set writes the config of a workspace: it creates it or replaces it, and
// whoever did it is kept.
func (s *PgStore) Set(ctx context.Context, w Workspace, actor string) error {
	if err := w.Valid(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO workspace.workspaces (owner_kind, owner_id, path, created_by)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (owner_kind, owner_id) DO UPDATE SET
		     path = EXCLUDED.path,
		     updated_at = goddard.now()`,
		w.Owner.Kind, w.Owner.ID, w.Path, actor)
	return err
}

// Delete takes the row of an owner out, so its projects go back to the volume
// by default. A row that is not there is not an error: what is left after this
// is the same either way. The row is config and not history, so it goes for
// real and nothing is left pointing at it.
func (s *PgStore) Delete(ctx context.Context, owner Owner) error {
	if err := checkOwner(owner); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM workspace.workspaces WHERE owner_kind = $1 AND owner_id = $2`,
		owner.Kind, owner.ID)
	return err
}
