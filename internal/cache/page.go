package cache

import (
	"encoding/json"
	"time"

	"taskservice/internal/domain"
)

type cachedPage struct {
	Items      []cachedTask `json:"items"`
	Limit      int          `json:"limit"`
	Offset     *int         `json:"offset,omitempty"`
	Total      *int64       `json:"total,omitempty"`
	NextCursor *string      `json:"next_cursor,omitempty"`
}

type cachedTask struct {
	ID          int64      `json:"id"`
	TeamID      int64      `json:"team_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	CreatedBy   int64      `json:"created_by"`
	AssigneeID  *int64     `json:"assignee_id,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
	Version     int        `json:"version"`
}

func encodePage(p domain.TaskPage) ([]byte, error) {
	out := cachedPage{Limit: p.Limit, Offset: p.Offset, Total: p.Total, NextCursor: p.NextCursor}
	out.Items = make([]cachedTask, len(p.Items))
	for i, t := range p.Items {
		out.Items[i] = cachedTask{
			ID: t.ID, TeamID: t.TeamID, Title: t.Title, Description: t.Description,
			Status: string(t.Status), CreatedBy: t.CreatedBy, AssigneeID: t.AssigneeID,
			CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, ClosedAt: t.ClosedAt, Version: t.Version,
		}
	}
	return json.Marshal(out)
}

func decodePage(raw []byte) (domain.TaskPage, error) {
	var in cachedPage
	if err := json.Unmarshal(raw, &in); err != nil {
		return domain.TaskPage{}, err
	}
	p := domain.TaskPage{Limit: in.Limit, Offset: in.Offset, Total: in.Total, NextCursor: in.NextCursor}
	p.Items = make([]domain.Task, len(in.Items))
	for i, t := range in.Items {
		p.Items[i] = domain.Task{
			ID: t.ID, TeamID: t.TeamID, Title: t.Title, Description: t.Description,
			Status: domain.TaskStatus(t.Status), CreatedBy: t.CreatedBy, AssigneeID: t.AssigneeID,
			CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, ClosedAt: t.ClosedAt, Version: t.Version,
		}
	}
	return p, nil
}
