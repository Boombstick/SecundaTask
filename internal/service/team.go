package service

import (
	"context"
	"strings"

	"taskservice/internal/domain"
	"taskservice/internal/platform/apperr"
	"taskservice/internal/platform/validate"
)

type TeamStore interface {
	Create(ctx context.Context, t *domain.Team) error
	ListForUser(ctx context.Context, userID int64) ([]domain.Team, error)
	Delete(ctx context.Context, id int64) error
	AddMember(ctx context.Context, m *domain.Member) error
	GetMember(ctx context.Context, teamID, userID int64) (domain.Member, error)
	ListMembers(ctx context.Context, teamID int64) ([]domain.Member, error)
	UpdateMemberRole(ctx context.Context, teamID, userID int64, role domain.Role) error
	DeleteMember(ctx context.Context, teamID, userID int64) error
}

type TaskUnassigner interface {
	UnassignMember(ctx context.Context, teamID, userID, changedBy int64) (int, error)
}

type TeamService struct {
	teams   TeamStore
	members *Membership
	users   UserStore
	tasks   TaskUnassigner
	cache   TaskListCache
	tx      Transactor
}

func NewTeamService(teams TeamStore, members *Membership, users UserStore, tasks TaskUnassigner, cache TaskListCache, tx Transactor) *TeamService {
	if teams == nil || members == nil || users == nil || tasks == nil || cache == nil || tx == nil {
		panic("service: NewTeamService: all dependencies are required (build TaskService first)")
	}
	return &TeamService{teams: teams, members: members, users: users, tasks: tasks, cache: cache, tx: tx}
}

func (s *TeamService) Create(ctx context.Context, userID int64, name string) (domain.Team, error) {
	name = strings.TrimSpace(name)
	if err := validate.ShortText("name", name); err != nil {
		return domain.Team{}, err
	}
	t := domain.Team{Name: name, CreatedBy: userID, Role: domain.RoleOwner}
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.teams.Create(ctx, &t); err != nil {
			return err
		}
		return s.teams.AddMember(ctx, &domain.Member{TeamID: t.ID, UserID: userID, Role: domain.RoleOwner})
	})
	return t, err
}

func (s *TeamService) List(ctx context.Context, userID int64) ([]domain.Team, error) {
	return s.teams.ListForUser(ctx, userID)
}

func (s *TeamService) Members(ctx context.Context, userID, teamID int64) ([]domain.Member, error) {
	if _, err := s.members.Actor(ctx, teamID, userID); err != nil {
		return nil, err
	}
	return s.teams.ListMembers(ctx, teamID)
}

func (s *TeamService) Invite(ctx context.Context, userID, teamID, inviteeID int64, role domain.Role) (domain.Member, error) {
	if role == "" {
		role = domain.RoleMember
	}
	a, err := s.members.Actor(ctx, teamID, userID)
	if err != nil {
		return domain.Member{}, err
	}
	var v apperr.ValidationErrors
	if inviteeID <= 0 {
		v.Add("user_id", "user_id is required")
	}
	if !role.Valid() {
		v.Add("role", "role must be one of: admin, member")
	}
	if err := v.Err(); err != nil {
		return domain.Member{}, err
	}
	if err := CanInvite(a, role); err != nil {
		return domain.Member{}, err
	}
	if _, err := s.users.GetByID(ctx, inviteeID); err != nil {
		return domain.Member{}, err
	}
	m := domain.Member{TeamID: teamID, UserID: inviteeID, Role: role}
	if err := s.teams.AddMember(ctx, &m); err != nil {
		return domain.Member{}, err
	}
	return s.teams.GetMember(ctx, teamID, inviteeID)
}

func (s *TeamService) ChangeRole(ctx context.Context, userID, teamID, targetID int64, role domain.Role) (domain.Member, error) {
	var out domain.Member
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		a, err := s.members.Actor(ctx, teamID, userID)
		if err != nil {
			return err
		}
		target, err := s.teams.GetMember(ctx, teamID, targetID)
		if err != nil {
			return err
		}
		if err := CanChangeRole(a, target, role); err != nil {
			return err
		}
		if err := s.teams.UpdateMemberRole(ctx, teamID, targetID, role); err != nil {
			return err
		}
		target.Role = role
		out = target
		return nil
	})
	return out, err
}

func (s *TeamService) RemoveMember(ctx context.Context, userID, teamID, targetID int64) error {
	var unassigned int
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		a, err := s.members.Actor(ctx, teamID, userID)
		if err != nil {
			return err
		}
		target, err := s.teams.GetMember(ctx, teamID, targetID)
		if err != nil {
			return err
		}
		if err := CanRemoveMember(a, target); err != nil {
			return err
		}
		if err := s.teams.DeleteMember(ctx, teamID, targetID); err != nil {
			return err
		}
		unassigned, err = s.tasks.UnassignMember(ctx, teamID, targetID, userID)
		return err
	})
	if err == nil && unassigned > 0 {
		s.cache.InvalidateTeam(ctx, teamID)
	}
	return err
}

func (s *TeamService) Delete(ctx context.Context, userID, teamID int64) error {
	a, err := s.members.Actor(ctx, teamID, userID)
	if err != nil {
		return err
	}
	if err := CanDeleteTeam(a); err != nil {
		return err
	}
	if err := s.teams.Delete(ctx, teamID); err != nil {
		return err
	}
	s.cache.InvalidateTeam(ctx, teamID)
	return nil
}
