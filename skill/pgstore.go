package skill

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"slices"
)

// PgStore keeps the skills in the skill schema of the shared database (see
// migrations/), over the same pool as the rest of goddard.
type PgStore struct {
	db *sql.DB
}

func NewPgStore(db *sql.DB) *PgStore {
	return &PgStore{db: db}
}

const metaColumns = `owner_kind, owner_id, role, name, description, updated_at`

const skillColumns = metaColumns + `, body, created_by, created_at`

const visible = `(owner_kind = 'system' OR (owner_kind = 'org' AND owner_id = $1) OR (owner_kind = 'user' AND owner_id = $2)) AND (role = '' OR role = $3)`

// List is every skill this viewer can see, one row per name, the closest
// owner's, in alphabetical order.
func (s *PgStore) List(ctx context.Context, viewer Viewer) ([]Meta, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+metaColumns+` FROM skill.skills WHERE `+visible,
		viewer.Org, viewer.User, viewer.Role)
	if err != nil {
		return nil, fmt.Errorf("no pude listar los skills: %w", err)
	}
	defer rows.Close()
	metas := []Meta{}
	for rows.Next() {
		var meta Meta
		if err := rows.Scan(&meta.Owner.Kind, &meta.Owner.ID, &meta.Role, &meta.Name, &meta.Description, &meta.UpdatedAt); err != nil {
			return nil, fmt.Errorf("no pude leer un skill: %w", err)
		}
		metas = append(metas, meta)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("no pude listar los skills: %w", err)
	}
	return closest(metas), nil
}

// Index is the list the system prompt shows.
func (s *PgStore) Index(ctx context.Context, viewer Viewer) (string, error) {
	metas, err := s.List(ctx, viewer)
	if err != nil {
		return "", err
	}
	return Index(metas), nil
}

// Load is the skill as the model reads it: the closest one this viewer can see.
func (s *PgStore) Load(ctx context.Context, viewer Viewer, name string) (string, error) {
	found, ok, err := s.Get(ctx, viewer, name)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("no existe la skill `%s`", name)
	}
	return Load(found), nil
}

// Get is the whole skill, body included.
func (s *PgStore) Get(ctx context.Context, viewer Viewer, name string) (Skill, bool, error) {
	if err := validateName(name); err != nil {
		return Skill{}, false, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+skillColumns+` FROM skill.skills WHERE name = $4 AND `+visible,
		viewer.Org, viewer.User, viewer.Role, name)
	if err != nil {
		return Skill{}, false, fmt.Errorf("no pude leer la skill %q: %w", name, err)
	}
	defer rows.Close()
	found := []Skill{}
	for rows.Next() {
		var one Skill
		if err := rows.Scan(&one.Owner.Kind, &one.Owner.ID, &one.Role, &one.Name,
			&one.Description, &one.UpdatedAt, &one.Body, &one.CreatedBy, &one.CreatedAt); err != nil {
			return Skill{}, false, fmt.Errorf("no pude leer la skill %q: %w", name, err)
		}
		found = append(found, one)
	}
	if err := rows.Err(); err != nil {
		return Skill{}, false, fmt.Errorf("no pude leer la skill %q: %w", name, err)
	}
	if len(found) == 0 {
		return Skill{}, false, nil
	}
	slices.SortStableFunc(found, func(a, b Skill) int {
		return cmp.Compare(rank(b.Owner.Kind), rank(a.Owner.Kind))
	})
	return found[0], true, nil
}

// Put writes a skill, creating it or replacing what the same owner had under
// that name. The system's are read only and belong to no viewer.
func (s *PgStore) Put(ctx context.Context, viewer Viewer, skill Skill) error {
	if err := validateName(skill.Name); err != nil {
		return err
	}
	if err := canWrite(viewer, skill.Owner); err != nil {
		return err
	}
	now := nowMs()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO skill.skills (owner_kind, owner_id, role, name, description, body, created_by, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (owner_kind, owner_id, name) DO UPDATE SET
		     role = EXCLUDED.role,
		     description = EXCLUDED.description,
		     body = EXCLUDED.body,
		     updated_at = EXCLUDED.updated_at`,
		skill.Owner.Kind, skill.Owner.ID, skill.Role, skill.Name, skill.Description,
		skill.Body, viewer.User, now, now)
	if err != nil {
		return fmt.Errorf("no pude guardar la skill %q: %w", skill.Name, err)
	}
	return nil
}

// Delete drops the skill that owner had under that name, and says whether
// there was one.
func (s *PgStore) Delete(ctx context.Context, viewer Viewer, owner Owner, name string) (bool, error) {
	if err := validateName(name); err != nil {
		return false, err
	}
	if err := canWrite(viewer, owner); err != nil {
		return false, err
	}
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM skill.skills WHERE owner_kind = $1 AND owner_id = $2 AND name = $3`,
		owner.Kind, owner.ID, name)
	if err != nil {
		return false, fmt.Errorf("no pude borrar la skill %q: %w", name, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("no pude borrar la skill %q: %w", name, err)
	}
	return affected > 0, nil
}

// closest keeps one row per name, the one of the closest owner, in
// alphabetical order.
func closest(metas []Meta) []Meta {
	slices.SortStableFunc(metas, func(a, b Meta) int {
		return cmp.Compare(rank(b.Owner.Kind), rank(a.Owner.Kind))
	})
	seen := map[string]bool{}
	kept := make([]Meta, 0, len(metas))
	for _, meta := range metas {
		if seen[meta.Name] {
			continue
		}
		seen[meta.Name] = true
		kept = append(kept, meta)
	}
	slices.SortFunc(kept, func(a, b Meta) int { return cmp.Compare(a.Name, b.Name) })
	return kept
}
