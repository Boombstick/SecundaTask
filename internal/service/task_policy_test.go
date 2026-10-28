package service

import (
	"testing"

	"taskservice/internal/domain"
	"taskservice/internal/platform/apperr"
)

var (
	owner  = domain.Actor{UserID: 1, Role: domain.RoleOwner}
	admin  = domain.Actor{UserID: 2, Role: domain.RoleAdmin}
	member = domain.Actor{UserID: 3, Role: domain.RoleMember}
	other  = domain.Actor{UserID: 4, Role: domain.RoleMember}
)

func TestCanDeleteTask(t *testing.T) {
	task := domain.Task{CreatedBy: member.UserID, AssigneeID: ptr(other.UserID)}
	cases := []struct {
		name  string
		actor domain.Actor
		want  apperr.Kind
	}{
		{"owner", owner, 0},
		{"admin", admin, 0},
		{"creator", member, 0},
		{"assignee cannot delete", other, apperr.KindForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := kindOf(CanDeleteTask(c.actor, task)); got != c.want {
				t.Errorf("got kind %d, want %d", got, c.want)
			}
		})
	}
}

func TestCanUpdateTask(t *testing.T) {
	current := domain.Task{
		Title:       "title",
		Description: "desc",
		Status:      domain.StatusBacklog,
		CreatedBy:   member.UserID,
		AssigneeID:  ptr(other.UserID),
	}
	same := TaskUpdate{Title: "title", Description: "desc", Status: domain.StatusBacklog, AssigneeID: ptr(other.UserID)}
	with := func(f func(*TaskUpdate)) TaskUpdate {
		u := same
		f(&u)
		return u
	}
	stranger := domain.Actor{UserID: 99, Role: domain.RoleMember}

	cases := []struct {
		name  string
		actor domain.Actor
		upd   TaskUpdate
		want  apperr.Kind
	}{
		{"owner edits anything", owner, with(func(u *TaskUpdate) { u.Title = "x"; u.AssigneeID = nil }), 0},
		{"admin edits anything", admin, with(func(u *TaskUpdate) { u.Description = "x"; u.AssigneeID = ptr(1) }), 0},
		{"creator edits title", member, with(func(u *TaskUpdate) { u.Title = "x" }), 0},
		{"creator reassigns", member, with(func(u *TaskUpdate) { u.AssigneeID = ptr(1) }), 0},
		{"creator unassigns", member, with(func(u *TaskUpdate) { u.AssigneeID = nil }), 0},
		{"assignee changes status", other, with(func(u *TaskUpdate) { u.Status = domain.StatusDone }), 0},
		{"assignee cannot change title", other, with(func(u *TaskUpdate) { u.Title = "x" }), apperr.KindForbidden},
		{"assignee cannot change description", other, with(func(u *TaskUpdate) { u.Description = "x" }), apperr.KindForbidden},
		{"assignee cannot reassign", other, with(func(u *TaskUpdate) { u.AssigneeID = ptr(1) }), apperr.KindForbidden},
		{"assignee cannot unassign", other, with(func(u *TaskUpdate) { u.AssigneeID = nil }), apperr.KindForbidden},
		{"stranger member cannot edit", stranger, with(func(u *TaskUpdate) { u.Status = domain.StatusDone }), apperr.KindForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := kindOf(CanUpdateTask(c.actor, current, c.upd)); got != c.want {
				t.Errorf("got kind %d, want %d", got, c.want)
			}
		})
	}
}

func TestCanUpdateTask_CreatorAndAssigneeCombined(t *testing.T) {
	self := domain.Actor{UserID: 7, Role: domain.RoleMember}
	task := domain.Task{Title: "t", CreatedBy: 7, AssigneeID: ptr(7)}
	upd := TaskUpdate{Title: "new", AssigneeID: ptr(8)}
	if err := CanUpdateTask(self, task, upd); err != nil {
		t.Fatalf("creator+assignee should have creator rights, got %v", err)
	}
}
