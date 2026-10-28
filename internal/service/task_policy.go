package service

import (
	"taskservice/internal/domain"
	"taskservice/internal/platform/apperr"
)

func CanDeleteTask(a domain.Actor, t domain.Task) error {
	if a.Role.IsManager() || t.CreatedBy == a.UserID {
		return nil
	}
	return apperr.Forbidden("only the team owner, an admin or the task creator can delete the task")
}

type TaskUpdate struct {
	Title       string
	Description string
	Status      domain.TaskStatus
	AssigneeID  *int64
}

func CanUpdateTask(a domain.Actor, current domain.Task, upd TaskUpdate) error {
	if a.Role.IsManager() || current.CreatedBy == a.UserID {
		return nil
	}
	if current.AssigneeID == nil || *current.AssigneeID != a.UserID {
		return apperr.Forbidden("you can edit only tasks you created or are assigned to")
	}
	if upd.Title != current.Title || upd.Description != current.Description {
		return apperr.Forbidden("the assignee can change only the task status")
	}
	if !domain.SameID(upd.AssigneeID, current.AssigneeID) {
		return apperr.Forbidden("the assignee cannot reassign the task")
	}
	return nil
}
