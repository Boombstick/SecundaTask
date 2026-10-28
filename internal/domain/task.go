package domain

import (
	"slices"
	"time"

	"taskservice/internal/platform/apperr"
)

type TaskStatus string

const (
	StatusBacklog    TaskStatus = "backlog"
	StatusInProgress TaskStatus = "in_progress"
	StatusNeedInfo   TaskStatus = "need_info"
	StatusDone       TaskStatus = "done"
)

var AllStatuses = []TaskStatus{StatusBacklog, StatusInProgress, StatusNeedInfo, StatusDone}

func (s TaskStatus) Valid() bool { return slices.Contains(AllStatuses, s) }

type Task struct {
	ID          int64
	TeamID      int64
	Title       string
	Description string
	Status      TaskStatus
	CreatedBy   int64
	AssigneeID  *int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ClosedAt    *time.Time
	Version     int
}

type Entity struct {
	ID   int64
	Name string
}

type TaskCard struct {
	Task
	Team     Entity
	Creator  Entity
	Assignee *Entity
}

type TaskFilter struct {
	TeamID     int64
	Status     *TaskStatus
	AssigneeID *int64
	Limit      int
	Offset     int
	AfterID    *int64
}

type TaskPage struct {
	Items      []Task
	Limit      int
	Offset     *int
	Total      *int64
	NextCursor *string
}

var (
	ErrTaskNotFound    = apperr.NotFound("task_not_found", "task not found")
	ErrVersionConflict = apperr.Conflict("version_conflict", "task was modified by someone else, reload it and retry")
)

func SameID(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
