package domain

import (
	"time"

	"taskservice/internal/platform/apperr"
)

type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

func (r Role) Valid() bool {
	return r == RoleOwner || r == RoleAdmin || r == RoleMember
}

func (r Role) IsManager() bool {
	return r == RoleOwner || r == RoleAdmin
}

type Team struct {
	ID        int64
	Name      string
	CreatedBy int64
	CreatedAt time.Time
	Role      Role
}

type Member struct {
	TeamID    int64
	UserID    int64
	Role      Role
	Email     string
	Name      string
	CreatedAt time.Time
}

type Actor struct {
	UserID int64
	Role   Role
}

var (
	ErrTeamNotFound   = apperr.NotFound("team_not_found", "team not found")
	ErrMemberNotFound = apperr.NotFound("member_not_found", "user is not a member of the team")
	ErrAlreadyMember  = apperr.Conflict("already_member", "user is already a member of the team")
)
