package org

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/3-lines-studio/goddard/naming"
)

// PgStore keeps the organizations and their members in the org schema of the
// shared database (see migrations/), over the same pool as the rest of goddard.
type PgStore struct {
	db *sql.DB
}

func NewPgStore(db *sql.DB) *PgStore {
	return &PgStore{db: db}
}

// queryer is what a query needs, so the same one reads inside a transaction
// and outside of it.
type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

const orgColumns = "o.id, o.slug, o.name, o.created_by"

// Orgs is every organization this user is in, by name.
func (s *PgStore) Orgs(ctx context.Context, userID string) ([]Org, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+orgColumns+" FROM org.orgs o WHERE o.deleted_at IS NULL AND EXISTS ("+
			"SELECT 1 FROM org.members m WHERE m.org_id = o.id AND m.user_id = $1 AND m.deleted_at IS NULL) "+
			"ORDER BY o.name, o.id", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	orgs := []Org{}
	for rows.Next() {
		var one Org
		if err := rows.Scan(&one.ID, &one.Slug, &one.Name, &one.CreatedBy); err != nil {
			return nil, err
		}
		orgs = append(orgs, one)
	}
	return orgs, rows.Err()
}

// Org finds one by id.
func (s *PgStore) Org(ctx context.Context, id string) (Org, bool, error) {
	var one Org
	err := s.db.QueryRowContext(ctx,
		"SELECT "+orgColumns+" FROM org.orgs o WHERE o.id = $1 AND o.deleted_at IS NULL", id).
		Scan(&one.ID, &one.Slug, &one.Name, &one.CreatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return Org{}, false, nil
	}
	if err != nil {
		return Org{}, false, err
	}
	return one, true, nil
}

// Create opens one, with whoever asked as its owner. The slug comes from the
// name, the same way a project takes its own, and is taken once.
func (s *PgStore) Create(ctx context.Context, name, createdBy string) (Org, error) {
	slug := naming.From(name)
	if slug == "" {
		return Org{}, errors.New("esa organización no tiene nombre")
	}
	created := Org{Slug: slug, Name: name, CreatedBy: createdBy}
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx,
			"INSERT INTO org.orgs (slug, name, created_by) VALUES ($1, $2, $3) RETURNING id",
			slug, name, createdBy).Scan(&created.ID)
		if taken(err) {
			return ErrTaken
		}
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			"INSERT INTO org.members (org_id, user_id, role) VALUES ($1, $2, $3)",
			created.ID, createdBy, RoleOwner)
		return err
	})
	if err != nil {
		return Org{}, err
	}
	return created, nil
}

// Members is who is in one, with the email and the name they came in with. The
// owners come first, then the admins, then the rest.
func (s *PgStore) Members(ctx context.Context, orgID string) ([]Member, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.user_id, u.email, u.name, m.role FROM org.members m
		 JOIN auth.users u ON u.id = m.user_id
		 WHERE m.org_id = $1 AND m.deleted_at IS NULL
		 ORDER BY CASE m.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END, u.name, m.user_id`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := []Member{}
	for rows.Next() {
		var member Member
		if err := rows.Scan(&member.UserID, &member.Email, &member.Name, &member.Role); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

// Role is what this user can do in this organization, if they are in it.
func (s *PgStore) Role(ctx context.Context, orgID, userID string) (string, bool, error) {
	return roleOf(ctx, s.db, orgID, userID)
}

// Add puts somebody in, by the email they came in with. An owner brings in
// anybody, an admin brings in members and admins, and nobody but an owner
// makes another owner.
func (s *PgStore) Add(ctx context.Context, orgID, email, role, actorID string) error {
	if !ValidRole(role) {
		return ErrRole
	}
	email = strings.ToLower(strings.TrimSpace(email))
	return s.inTx(ctx, func(tx *sql.Tx) error {
		actor, err := member(ctx, tx, orgID, actorID, true)
		if err != nil {
			return err
		}
		if !canInvite(actor) || (role == RoleOwner && actor != RoleOwner) {
			return ErrForbidden
		}
		var userID string
		err = tx.QueryRowContext(ctx,
			"SELECT id FROM auth.users WHERE lower(email) = $1 AND deleted_at IS NULL", email).Scan(&userID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNoUser
		}
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			"INSERT INTO org.members (org_id, user_id, role) VALUES ($1, $2, $3) "+
				"ON CONFLICT ON CONSTRAINT members_natural DO UPDATE SET role = $3, deleted_at = NULL, updated_at = goddard.now()",
			orgID, userID, role)
		return err
	})
}

// Remove takes somebody out. An owner takes out anybody but the last owner; an
// admin takes out members, and only members.
func (s *PgStore) Remove(ctx context.Context, orgID, userID, actorID string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		actor, err := member(ctx, tx, orgID, actorID, true)
		if err != nil {
			return err
		}
		if !canInvite(actor) {
			return ErrForbidden
		}
		target, err := member(ctx, tx, orgID, userID, false)
		if err != nil {
			return err
		}
		if target == RoleOwner && actor != RoleOwner {
			return ErrForbidden
		}
		if actor == RoleAdmin && target != RoleMember {
			return ErrForbidden
		}
		if target == RoleOwner {
			if err := s.lastOwner(ctx, tx, orgID); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx,
			"UPDATE org.members SET deleted_at = goddard.now(), updated_at = goddard.now() "+
				"WHERE org_id = $1 AND user_id = $2 AND deleted_at IS NULL", orgID, userID)
		return err
	})
}

// SetRole changes what somebody can do. Only an owner does it, and the last
// owner stays an owner.
func (s *PgStore) SetRole(ctx context.Context, orgID, userID, role, actorID string) error {
	if !ValidRole(role) {
		return ErrRole
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		actor, err := member(ctx, tx, orgID, actorID, true)
		if err != nil {
			return err
		}
		if actor != RoleOwner {
			return ErrForbidden
		}
		target, err := member(ctx, tx, orgID, userID, false)
		if err != nil {
			return err
		}
		if target == RoleOwner && role != RoleOwner {
			if err := s.lastOwner(ctx, tx, orgID); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx,
			"UPDATE org.members SET role = $3, updated_at = goddard.now() "+
				"WHERE org_id = $1 AND user_id = $2 AND deleted_at IS NULL", orgID, userID, role)
		return err
	})
}

// Delete takes the organization out, with the memberships. Only an owner asks
// for it. It is marked, not dropped, the way a project is: what happened keeps
// pointing at it. The projects are not this store's to take: they belong to
// chat and go before this, so a failure here leaves an organization nobody can
// see instead of projects nobody can reach.
func (s *PgStore) Delete(ctx context.Context, orgID, actorID string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		role, err := member(ctx, tx, orgID, actorID, true)
		if err != nil {
			return err
		}
		if role != RoleOwner {
			return ErrForbidden
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE org.members SET deleted_at = goddard.now(), updated_at = goddard.now() "+
				"WHERE org_id = $1 AND deleted_at IS NULL", orgID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			"UPDATE org.orgs SET deleted_at = goddard.now(), updated_at = goddard.now() "+
				"WHERE id = $1 AND deleted_at IS NULL", orgID)
		return err
	})
}

// lastOwner refuses when this is the only owner left, so an organization never
// ends up with nobody who can bring people in.
func (s *PgStore) lastOwner(ctx context.Context, tx *sql.Tx, orgID string) error {
	var owners int
	if err := tx.QueryRowContext(ctx,
		"SELECT count(*) FROM org.members WHERE org_id = $1 AND role = $2 AND deleted_at IS NULL",
		orgID, RoleOwner).Scan(&owners); err != nil {
		return err
	}
	if owners <= 1 {
		return ErrLastOwner
	}
	return nil
}

// member reads the role of somebody in one, holding the organization for the
// rest of the transaction first so two people changing it at the same time
// take turns instead of racing over the last owner. A missing member is
// ErrForbidden for the one asking and ErrNoMember for the one being asked
// about: they are not the same thing to whoever reads the error.
func member(ctx context.Context, tx *sql.Tx, orgID, userID string, actor bool) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx,
		"SELECT id FROM org.orgs WHERE id = $1 AND deleted_at IS NULL FOR UPDATE", orgID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNoOrg
	}
	if err != nil {
		return "", err
	}
	role, ok, err := roleOf(ctx, tx, orgID, userID)
	if err != nil {
		return "", err
	}
	if !ok {
		if actor {
			return "", ErrForbidden
		}
		return "", ErrNoMember
	}
	return role, nil
}

// roleOf is the role of somebody in one, or nothing when they are not in it.
func roleOf(ctx context.Context, q queryer, orgID, userID string) (string, bool, error) {
	var role string
	err := q.QueryRowContext(ctx,
		"SELECT role FROM org.members WHERE org_id = $1 AND user_id = $2 AND deleted_at IS NULL",
		orgID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return role, true, nil
}

func (s *PgStore) inTx(ctx context.Context, work func(tx *sql.Tx) error) error {
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

func taken(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}
