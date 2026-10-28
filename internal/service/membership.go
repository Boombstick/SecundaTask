package service

import (
	"context"
	"errors"

	"taskservice/internal/domain"
)

type MemberStore interface {
	GetMember(ctx context.Context, teamID, userID int64) (domain.Member, error)
	LockMember(ctx context.Context, teamID, userID int64) (bool, error)
}

type Membership struct {
	members MemberStore
}

func NewMembership(members MemberStore) *Membership { return &Membership{members: members} }

func (m *Membership) Actor(ctx context.Context, teamID, userID int64) (domain.Actor, error) {
	mem, err := m.members.GetMember(ctx, teamID, userID)
	if errors.Is(err, domain.ErrMemberNotFound) {
		return domain.Actor{}, domain.ErrTeamNotFound
	}
	if err != nil {
		return domain.Actor{}, err
	}
	return domain.Actor{UserID: userID, Role: mem.Role}, nil
}

func (m *Membership) LockMember(ctx context.Context, teamID, userID int64) (bool, error) {
	return m.members.LockMember(ctx, teamID, userID)
}
