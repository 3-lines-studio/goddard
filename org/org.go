// Package org keeps the organizations of goddard: who is in one and what they
// can do. An organization owns projects, has a workspace of its own and shares
// its memory with its members, so being in one is how several people work on
// the same thing.
//
// Every organization has at least one owner, and the owner is the one who
// decides who else gets in and what they are. The roles are three: an owner
// does everything, an admin brings people in and takes them out, and a member
// works inside.
package org

import "errors"

// The roles a member can have.
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

var (
	ErrTaken     = errors.New("esa organización ya existe")
	ErrNoOrg     = errors.New("esa organización no existe")
	ErrNoUser    = errors.New("ese mail no es de nadie todavía")
	ErrNoMember  = errors.New("esa persona no está en la organización")
	ErrRole      = errors.New("ese rol no existe")
	ErrForbidden = errors.New("no podés hacer eso en esa organización")
	ErrLastOwner = errors.New("la organización se quedaría sin dueño")
)

// Org is an organization: its id names the directory of its workspace, the way
// the id of a person does.
type Org struct {
	ID        string `json:"id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	CreatedBy string `json:"created_by"`
}

// Member is somebody in an organization, with the email and the name they came
// in with.
type Member struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	Role   string `json:"role"`
}

// ValidRole says whether the role is one of the three.
func ValidRole(role string) bool {
	return role == RoleOwner || role == RoleAdmin || role == RoleMember
}

// canInvite is who may bring somebody in: an owner or an admin.
func canInvite(role string) bool {
	return role == RoleOwner || role == RoleAdmin
}
