package repository

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"

	"taskservice/internal/domain"
	"taskservice/internal/platform/database"
)

type taskModel struct {
	ID          int64 `gorm:"primaryKey"`
	TeamID      int64
	Title       string
	Description string
	Status      string
	CreatedBy   int64
	AssigneeID  *int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ClosedAt    *time.Time
	Version     int
}

func (taskModel) TableName() string { return "tasks" }

const taskColumns = `id, team_id, title, description, status, created_by, assignee_id,
	created_at, updated_at, closed_at, version`

type TaskRepository struct {
	db *database.DB
}

func NewTaskRepository(db *database.DB) *TaskRepository { return &TaskRepository{db: db} }

func (r *TaskRepository) Create(ctx context.Context, t *domain.Task) error {
	m := taskModel{
		TeamID:      t.TeamID,
		Title:       t.Title,
		Description: t.Description,
		Status:      string(t.Status),
		CreatedBy:   t.CreatedBy,
		AssigneeID:  t.AssigneeID,
		ClosedAt:    t.ClosedAt,
		Version:     1,
	}
	if err := r.db.Conn(ctx).Create(&m).Error; err != nil {
		return err
	}
	t.ID, t.CreatedAt, t.UpdatedAt, t.Version = m.ID, m.CreatedAt, m.UpdatedAt, m.Version
	return nil
}

func (r *TaskRepository) Get(ctx context.Context, id int64) (domain.Task, error) {
	var t domain.Task
	err := r.db.ScanOne(ctx, &t, domain.ErrTaskNotFound, `SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id)
	return t, err
}

func (r *TaskRepository) GetCard(ctx context.Context, id int64) (domain.TaskCard, error) {
	var row struct {
		domain.Task
		TeamName     string
		CreatorName  string
		AssigneeName *string
	}
	err := r.db.ScanOne(ctx, &row, domain.ErrTaskNotFound, `
		SELECT t.id, t.team_id, t.title, t.description, t.status, t.created_by, t.assignee_id,
		       t.created_at, t.updated_at, t.closed_at, t.version,
		       tm.name AS team_name, c.name AS creator_name, a.name AS assignee_name
		FROM tasks t
		JOIN teams tm ON tm.id = t.team_id
		JOIN users c ON c.id = t.created_by
		LEFT JOIN users a ON a.id = t.assignee_id
		WHERE t.id = ?`, id)
	if err != nil {
		return domain.TaskCard{}, err
	}
	card := domain.TaskCard{
		Task:    row.Task,
		Team:    domain.Entity{ID: row.TeamID, Name: row.TeamName},
		Creator: domain.Entity{ID: row.CreatedBy, Name: row.CreatorName},
	}
	if row.AssigneeID != nil && row.AssigneeName != nil {
		card.Assignee = &domain.Entity{ID: *row.AssigneeID, Name: *row.AssigneeName}
	}
	return card, nil
}

func (r *TaskRepository) Update(ctx context.Context, t *domain.Task, expectedVersion int) error {
	now := r.db.Now()
	res := r.db.Conn(ctx).Model(&taskModel{}).
		Where("id = ? AND version = ?", t.ID, expectedVersion).
		Updates(map[string]any{
			"title":       t.Title,
			"description": t.Description,
			"status":      string(t.Status),
			"assignee_id": t.AssigneeID,
			"closed_at":   t.ClosedAt,
			"updated_at":  now,
			"version":     gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrVersionConflict
	}
	t.Version = expectedVersion + 1
	t.UpdatedAt = now
	return nil
}

func (r *TaskRepository) Delete(ctx context.Context, id int64, expectedVersion *int) error {
	q := r.db.Conn(ctx).Where("id = ?", id)
	if expectedVersion != nil {
		q = q.Where("version = ?", *expectedVersion)
	}
	res := q.Delete(&taskModel{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		if expectedVersion != nil {
			return domain.ErrVersionConflict
		}
		return domain.ErrTaskNotFound
	}
	return nil
}

func (r *TaskRepository) List(ctx context.Context, f domain.TaskFilter) ([]domain.Task, *int64, error) {
	where := []string{"team_id = ?"}
	args := []any{f.TeamID}
	if f.Status != nil {
		where = append(where, "status = ?")
		args = append(args, string(*f.Status))
	}
	if f.AssigneeID != nil {
		where = append(where, "assignee_id = ?")
		args = append(args, *f.AssigneeID)
	}
	cond := strings.Join(where, " AND ")

	var total *int64
	if f.AfterID == nil {
		var n int64
		if err := r.db.Conn(ctx).Raw(`SELECT COUNT(*) FROM tasks WHERE `+cond, args...).Scan(&n).Error; err != nil {
			return nil, nil, err
		}
		total = &n
	}

	query := `SELECT ` + taskColumns + ` FROM tasks WHERE ` + cond
	if f.AfterID != nil {
		query += ` AND id < ?`
		args = append(args, *f.AfterID)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, f.Limit+1)
	if f.AfterID == nil {
		query += ` OFFSET ?`
		args = append(args, f.Offset)
	}

	tasks := []domain.Task{}
	if err := r.db.Conn(ctx).Raw(query, args...).Scan(&tasks).Error; err != nil {
		return nil, nil, err
	}
	return tasks, total, nil
}

func (r *TaskRepository) UnassignUser(ctx context.Context, teamID, userID int64) ([]int64, error) {
	var ids []int64
	if err := r.db.Conn(ctx).Raw(
		`SELECT id FROM tasks WHERE team_id = ? AND assignee_id = ? FOR UPDATE`, teamID, userID,
	).Scan(&ids).Error; err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	err := r.db.Conn(ctx).Model(&taskModel{}).
		Where("id IN ?", ids).
		Updates(map[string]any{
			"assignee_id": nil,
			"version":     gorm.Expr("version + 1"),
			"updated_at":  r.db.Now(),
		}).Error
	return ids, err
}
