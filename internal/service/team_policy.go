package service

import (
	"taskservice/internal/domain"
	"taskservice/internal/platform/apperr"
)

func CanInvite(a domain.Actor, role domain.Role) error {
	if !a.Role.IsManager() {
		return apperr.Forbidden("only the team owner or an admin can invite users")
	}
	switch role {
	case domain.RoleMember:
		return nil
	case domain.RoleAdmin:
		if a.Role != domain.RoleOwner {
			return apperr.Forbidden("only the team owner can grant the admin role")
		}
		return nil
	case domain.RoleOwner:
		return apperr.Forbidden("the owner role cannot be granted by invitation")
	default:
		return apperr.ValidationField("role", "role must be one of: admin, member")
	}
}

func CanChangeRole(a domain.Actor, target domain.Member, newRole domain.Role) error {
	if newRole != domain.RoleAdmin && newRole != domain.RoleMember {
		return apperr.ValidationField("role", "role must be one of: admin, member")
	}
	if a.Role != domain.RoleOwner {
		return apperr.Forbidden("only the team owner can change roles")
	}
	if target.Role == domain.RoleOwner {
		return apperr.Forbidden("the owner's role cannot be changed")
	}
	return nil
}

func CanRemoveMember(a domain.Actor, target domain.Member) error {
	if target.Role == domain.RoleOwner {
		return apperr.Forbidden("the team owner cannot be removed")
	}
	if a.UserID == target.UserID {
		return nil
	}
	switch a.Role {
	case domain.RoleOwner:
		return nil
	case domain.RoleAdmin:
		if target.Role == domain.RoleMember {
			return nil
		}
		return apperr.Forbidden("only the team owner can remove an admin")
	default:
		return apperr.Forbidden("only the team owner or an admin can remove members")
	}
}

func CanDeleteTeam(a domain.Actor) error {
	if a.Role != domain.RoleOwner {
		return apperr.Forbidden("only the team owner can delete the team")
	}
	return nil
}
