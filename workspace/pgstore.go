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

const columns = `path, sandbox_kind, sandbox_addr, sandbox_user`

// Get is the workspace of an owner: the row, or the default of that owner when
// there is no row yet, so a workspace exists without anybody creating it.
func (s *PgStore) Get(ctx context.Context, owner Owner) (Workspace, error) {
	if err := checkOwner(owner); err != nil {
		return Workspace{}, err
	}
	found := New(owner)
	err := s.db.QueryRowContext(ctx,
		`SELECT `+columns+` FROM workspace.workspaces WHERE owner_kind = $1 AND owner_id = $2`,
		owner.Kind, owner.ID).Scan(&found.Path, &found.Sandbox.Kind, &found.Sandbox.Addr, &found.Sandbox.User)
	if errors.Is(err, sql.ErrNoRows) {
		return New(owner), nil
	}
	if err != nil {
		return Workspace{}, err
	}
	return found, nil
}

// Set writes the config of a workspace: it creates it or replaces it, and
// whoever did it is kept.
func (s *PgStore) Set(ctx context.Context, w Workspace, actor string) error {
	if err := w.Valid(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO workspace.workspaces (owner_kind, owner_id, path, sandbox_kind, sandbox_addr, sandbox_user, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (owner_kind, owner_id) DO UPDATE SET
		     path = EXCLUDED.path,
		     sandbox_kind = EXCLUDED.sandbox_kind,
		     sandbox_addr = EXCLUDED.sandbox_addr,
		     sandbox_user = EXCLUDED.sandbox_user,
		     updated_at = goddard.now()`,
		w.Owner.Kind, w.Owner.ID, w.Path, w.Sandbox.Kind, w.Sandbox.Addr, w.Sandbox.User, actor)
	return err
}
