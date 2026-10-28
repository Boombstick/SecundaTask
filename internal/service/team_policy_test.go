package service

import (
	"testing"

	"taskservice/internal/domain"
	"taskservice/internal/platform/apperr"
)

var (
	tOwner  = domain.Actor{UserID: 1, Role: domain.RoleOwner}
	tAdmin  = domain.Actor{UserID: 2, Role: domain.RoleAdmin}
	tMember = domain.Actor{UserID: 3, Role: domain.RoleMember}
)

func TestCanInvite(t *testing.T) {
	cases := []struct {
		name  string
		actor domain.Actor
		role  domain.Role
		want  apperr.Kind
	}{
		{"owner invites member", tOwner, domain.RoleMember, 0},
		{"owner invites admin", tOwner, domain.RoleAdmin, 0},
		{"owner cannot invite owner", tOwner, domain.RoleOwner, apperr.KindForbidden},
		{"admin invites member", tAdmin, domain.RoleMember, 0},
		{"admin cannot invite admin", tAdmin, domain.RoleAdmin, apperr.KindForbidden},
		{"member cannot invite", tMember, domain.RoleMember, apperr.KindForbidden},
		{"unknown role", tOwner, domain.Role("boss"), apperr.KindValidation},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := kindOf(CanInvite(c.actor, c.role)); got != c.want {
				t.Errorf("got kind %d, want %d", got, c.want)
			}
		})
	}
}

func TestCanChangeRole(t *testing.T) {
	ownerM := domain.Member{UserID: 1, Role: domain.RoleOwner}
	adminM := domain.Member{UserID: 2, Role: domain.RoleAdmin}
	memberM := domain.Member{UserID: 3, Role: domain.RoleMember}
	cases := []struct {
		name    string
		actor   domain.Actor
		target  domain.Member
		newRole domain.Role
		want    apperr.Kind
	}{
		{"owner promotes member", tOwner, memberM, domain.RoleAdmin, 0},
		{"owner demotes admin", tOwner, adminM, domain.RoleMember, 0},
		{"owner cannot change own role", tOwner, ownerM, domain.RoleAdmin, apperr.KindForbidden},
		{"cannot grant owner", tOwner, memberM, domain.RoleOwner, apperr.KindValidation},
		{"admin cannot change roles", tAdmin, memberM, domain.RoleAdmin, apperr.KindForbidden},
		{"admin cannot change owner", tAdmin, ownerM, domain.RoleMember, apperr.KindForbidden},
		{"member cannot change roles", tMember, memberM, domain.RoleAdmin, apperr.KindForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := kindOf(CanChangeRole(c.actor, c.target, c.newRole)); got != c.want {
				t.Errorf("got kind %d, want %d", got, c.want)
			}
		})
	}
}

func TestCanRemoveMember(t *testing.T) {
	ownerM := domain.Member{UserID: 1, Role: domain.RoleOwner}
	adminM := domain.Member{UserID: 2, Role: domain.RoleAdmin}
	admin2M := domain.Member{UserID: 5, Role: domain.RoleAdmin}
	memberM := domain.Member{UserID: 3, Role: domain.RoleMember}
	otherM := domain.Member{UserID: 4, Role: domain.RoleMember}
	cases := []struct {
		name   string
		actor  domain.Actor
		target domain.Member
		want   apperr.Kind
	}{
		{"owner removes admin", tOwner, adminM, 0},
		{"owner removes member", tOwner, memberM, 0},
		{"owner cannot remove self", tOwner, ownerM, apperr.KindForbidden},
		{"admin cannot remove owner", tAdmin, ownerM, apperr.KindForbidden},
		{"admin removes member", tAdmin, memberM, 0},
		{"admin cannot remove another admin", tAdmin, admin2M, apperr.KindForbidden},
		{"admin leaves", tAdmin, adminM, 0},
		{"member leaves", tMember, memberM, 0},
		{"member cannot remove others", tMember, otherM, apperr.KindForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := kindOf(CanRemoveMember(c.actor, c.target)); got != c.want {
				t.Errorf("got kind %d, want %d", got, c.want)
			}
		})
	}
}

func TestCanDeleteTeam(t *testing.T) {
	if err := CanDeleteTeam(tOwner); err != nil {
		t.Errorf("owner: unexpected error %v", err)
	}
	if kindOf(CanDeleteTeam(tAdmin)) != apperr.KindForbidden {
		t.Error("admin must not delete team")
	}
	if kindOf(CanDeleteTeam(tMember)) != apperr.KindForbidden {
		t.Error("member must not delete team")
	}
}
