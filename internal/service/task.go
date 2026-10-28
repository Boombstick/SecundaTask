package service

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"taskservice/internal/domain"
	"taskservice/internal/platform/apperr"
	"taskservice/internal/platform/validate"
)

type TaskStore interface {
	Create(ctx context.Context, t *domain.Task) error
	Get(ctx context.Context, id int64) (domain.Task, error)
	GetCard(ctx context.Context, id int64) (domain.TaskCard, error)
	Update(ctx context.Context, t *domain.Task, expectedVersion int) error
	Delete(ctx context.Context, id int64, expectedVersion *int) error
	List(ctx context.Context, f domain.TaskFilter) ([]domain.Task, *int64, error)
	UnassignUser(ctx context.Context, teamID, userID int64) ([]int64, error)
}

type TaskListCache interface {
	Get(ctx context.Context, teamID int64, f domain.TaskFilter) (domain.TaskPage, bool)
	Set(ctx context.Context, teamID int64, f domain.TaskFilter, page domain.TaskPage)
	InvalidateTeam(ctx context.Context, teamID int64)
}

type TaskService struct {
	tasks   TaskStore
	history HistoryStore
	members *Membership
	cache   TaskListCache
	tx      Transactor
	now     func() time.Time
}

func NewTaskService(tasks TaskStore, history HistoryStore, members *Membership, cache TaskListCache, tx Transactor) *TaskService {
	return &TaskService{
		tasks: tasks, history: history, members: members, cache: cache, tx: tx,
		now: func() time.Time { return time.Now().UTC() },
	}
}

type CreateTaskInput struct {
	TeamID      int64
	Title       string
	Description string
	AssigneeID  *int64
}

type UpdateTaskInput struct {
	Title       *string
	Description string
	Status      *domain.TaskStatus
	AssigneeID  *int64
	Version     *int
}

type ListTasksInput struct {
	TeamID     string
	Status     string
	AssigneeID string
	Limit      string
	Offset     string
	Cursor     string
}

func validateStatus(field string, s domain.TaskStatus) error {
	if s.Valid() {
		return nil
	}
	return apperr.ValidationField(field, field+" must be one of: backlog, in_progress, need_info, done")
}

func checkTaskFields(v *apperr.ValidationErrors, title, description string) {
	v.Check(validate.ShortText("title", title))
	v.Check(validate.LongText("description", description, 0))
}

func (s *TaskService) checkAssignee(ctx context.Context, v *apperr.ValidationErrors, teamID int64, assigneeID *int64) {
	if assigneeID == nil {
		return
	}
	ok, err := s.members.LockMember(ctx, teamID, *assigneeID)
	if err != nil {
		v.Check(err)
		return
	}
	if !ok {
		v.Add("assignee_id", "assignee must be a member of the task's team")
	}
}

func (s *TaskService) load(ctx context.Context, userID, taskID int64) (domain.Task, domain.Actor, error) {
	t, err := s.tasks.Get(ctx, taskID)
	if err != nil {
		return domain.Task{}, domain.Actor{}, err
	}
	a, err := s.taskActor(ctx, t.TeamID, userID)
	return t, a, err
}

func (s *TaskService) taskActor(ctx context.Context, teamID, userID int64) (domain.Actor, error) {
	a, err := s.members.Actor(ctx, teamID, userID)
	if errors.Is(err, domain.ErrTeamNotFound) {
		return domain.Actor{}, domain.ErrTaskNotFound
	}
	return a, err
}

func (s *TaskService) Card(ctx context.Context, userID, taskID int64) (domain.TaskCard, error) {
	card, err := s.tasks.GetCard(ctx, taskID)
	if err != nil {
		return domain.TaskCard{}, err
	}
	if _, err := s.taskActor(ctx, card.TeamID, userID); err != nil {
		return domain.TaskCard{}, err
	}
	return card, nil
}

func (s *TaskService) Create(ctx context.Context, userID int64, in CreateTaskInput) (domain.TaskCard, error) {
	in.Title = strings.TrimSpace(in.Title)
	var v apperr.ValidationErrors
	if in.TeamID <= 0 {
		v.Add("team_id", "team_id is required")
	} else if _, err := s.members.Actor(ctx, in.TeamID, userID); err != nil {
		return domain.TaskCard{}, err
	}
	checkTaskFields(&v, in.Title, in.Description)

	t := domain.Task{
		TeamID:      in.TeamID,
		Title:       in.Title,
		Description: in.Description,
		Status:      domain.StatusBacklog,
		CreatedBy:   userID,
		AssigneeID:  in.AssigneeID,
	}
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if in.TeamID > 0 {
			s.checkAssignee(ctx, &v, in.TeamID, in.AssigneeID)
		}
		if err := v.Err(); err != nil {
			return err
		}
		if err := s.tasks.Create(ctx, &t); err != nil {
			return err
		}
		return s.history.Add(ctx, t.ID, userID, domain.Changes{
			"title":       {Old: nil, New: t.Title},
			"description": {Old: nil, New: t.Description},
			"status":      {Old: nil, New: t.Status},
			"assignee_id": {Old: nil, New: t.AssigneeID},
		})
	})
	if err != nil {
		return domain.TaskCard{}, err
	}
	s.cache.InvalidateTeam(ctx, t.TeamID)
	return s.tasks.GetCard(ctx, t.ID)
}

func (s *TaskService) Get(ctx context.Context, userID, taskID int64) (domain.Task, error) {
	t, _, err := s.load(ctx, userID, taskID)
	return t, err
}

func (s *TaskService) Update(ctx context.Context, userID, taskID int64, in UpdateTaskInput) (domain.TaskCard, error) {
	var v apperr.ValidationErrors
	var title string
	if in.Title == nil {
		v.Add("title", "title is required")
	} else {
		title = strings.TrimSpace(*in.Title)
		v.Check(validate.ShortText("title", title))
	}
	v.Check(validate.LongText("description", in.Description, 0))
	if in.Status == nil {
		v.Add("status", "status is required")
	} else {
		v.Check(validateStatus("status", *in.Status))
	}
	if in.Version == nil || *in.Version <= 0 {
		v.Add("version", "version is required and must be positive")
	}

	var (
		updated domain.Task
		changed bool
	)
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		cur, a, err := s.load(ctx, userID, taskID)
		if err != nil {
			return err
		}
		if !domain.SameID(cur.AssigneeID, in.AssigneeID) {
			s.checkAssignee(ctx, &v, cur.TeamID, in.AssigneeID)
		}
		if err := v.Err(); err != nil {
			return err
		}
		upd := TaskUpdate{Title: title, Description: in.Description, Status: *in.Status, AssigneeID: in.AssigneeID}
		if err := CanUpdateTask(a, cur, upd); err != nil {
			return err
		}
		if cur.Version != *in.Version {
			return domain.ErrVersionConflict
		}

		next := cur
		next.Title, next.Description, next.Status, next.AssigneeID = upd.Title, upd.Description, upd.Status, upd.AssigneeID
		switch {
		case next.Status == domain.StatusDone && cur.Status != domain.StatusDone:
			now := s.now()
			next.ClosedAt = &now
		case next.Status != domain.StatusDone:
			next.ClosedAt = nil
		}

		changes := diffTask(cur, next)
		if len(changes) == 0 {
			updated = cur
			return nil
		}
		if err := s.tasks.Update(ctx, &next, cur.Version); err != nil {
			return err
		}
		changed = true
		updated = next
		return s.history.Add(ctx, cur.ID, userID, changes)
	})
	if err != nil {
		return domain.TaskCard{}, err
	}
	if changed {
		s.cache.InvalidateTeam(ctx, updated.TeamID)
	}
	return s.tasks.GetCard(ctx, updated.ID)
}

func (s *TaskService) Delete(ctx context.Context, userID, taskID int64, version *int) error {
	t, a, err := s.load(ctx, userID, taskID)
	if err != nil {
		return err
	}
	if err := CanDeleteTask(a, t); err != nil {
		return err
	}
	if err := s.tasks.Delete(ctx, taskID, version); err != nil {
		return err
	}
	s.cache.InvalidateTeam(ctx, t.TeamID)
	return nil
}

func (s *TaskService) UnassignMember(ctx context.Context, teamID, userID, changedBy int64) (int, error) {
	ids, err := s.tasks.UnassignUser(ctx, teamID, userID)
	if err != nil {
		return 0, err
	}
	changes := domain.Changes{"assignee_id": {Old: userID, New: nil}}
	if err := s.history.AddBatch(ctx, ids, changedBy, changes); err != nil {
		return 0, err
	}
	return len(ids), nil
}

func parseListInput(in ListTasksInput) (domain.TaskFilter, error) {
	var v apperr.ValidationErrors
	var f domain.TaskFilter

	if in.TeamID == "" {
		v.Add("team_id", "team_id is required")
	} else if id := validate.OptionalID(&v, "team_id", in.TeamID); id != nil {
		f.TeamID = *id
	}
	if in.Status != "" {
		st := domain.TaskStatus(in.Status)
		if v.Check(validateStatus("status", st)) {
			f.Status = &st
		}
	}
	f.AssigneeID = validate.OptionalID(&v, "assignee_id", in.AssigneeID)
	f.Limit, f.Offset = validate.Page(&v, validate.PageQuery{Limit: in.Limit, Offset: in.Offset})
	if in.Cursor != "" {
		if after, err := decodeCursor(in.Cursor); v.Check(err) {
			f.AfterID, f.Offset = &after, 0
		}
	}
	return f, v.Err()
}

func (s *TaskService) List(ctx context.Context, userID int64, in ListTasksInput) (domain.TaskPage, error) {
	f, err := parseListInput(in)
	if err != nil {
		return domain.TaskPage{}, err
	}
	if _, err := s.members.Actor(ctx, f.TeamID, userID); err != nil {
		return domain.TaskPage{}, err
	}

	if page, hit := s.cache.Get(ctx, f.TeamID, f); hit {
		return page, nil
	}

	tasks, total, err := s.tasks.List(ctx, f)
	if err != nil {
		return domain.TaskPage{}, err
	}
	page := domain.TaskPage{Items: tasks, Limit: f.Limit, Total: total}
	if f.AfterID == nil {
		page.Offset = &f.Offset
	}
	if len(tasks) > f.Limit {
		page.Items = tasks[:f.Limit]
		c := encodeCursor(page.Items[f.Limit-1].ID)
		page.NextCursor = &c
	}

	s.cache.Set(ctx, f.TeamID, f, page)
	return page, nil
}

func diffTask(cur, next domain.Task) domain.Changes {
	ch := domain.Changes{}
	if cur.Title != next.Title {
		ch["title"] = domain.FieldChange{Old: cur.Title, New: next.Title}
	}
	if cur.Description != next.Description {
		ch["description"] = domain.FieldChange{Old: cur.Description, New: next.Description}
	}
	if cur.Status != next.Status {
		ch["status"] = domain.FieldChange{Old: cur.Status, New: next.Status}
	}
	if !domain.SameID(cur.AssigneeID, next.AssigneeID) {
		ch["assignee_id"] = domain.FieldChange{Old: cur.AssigneeID, New: next.AssigneeID}
	}
	if !sameTime(cur.ClosedAt, next.ClosedAt) {
		ch["closed_at"] = domain.FieldChange{Old: cur.ClosedAt, New: next.ClosedAt}
	}
	return ch
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func encodeCursor(id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte("id:" + strconv.FormatInt(id, 10)))
}

func decodeCursor(c string) (int64, error) {
	invalid := apperr.ValidationField("cursor", "cursor is invalid")
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return 0, invalid
	}
	v, ok := strings.CutPrefix(string(raw), "id:")
	if !ok {
		return 0, invalid
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		return 0, invalid
	}
	return id, nil
}
