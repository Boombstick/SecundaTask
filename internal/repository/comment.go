package repository

import (
	"context"
	"time"

	"taskservice/internal/domain"
	"taskservice/internal/platform/database"
)

type commentModel struct {
	ID        int64 `gorm:"primaryKey"`
	TaskID    int64
	UserID    int64
	Content   string
	CreatedAt time.Time
}

func (commentModel) TableName() string { return "task_comments" }

type CommentRepository struct {
	db *database.DB
}

func NewCommentRepository(db *database.DB) *CommentRepository { return &CommentRepository{db: db} }

func (r *CommentRepository) Create(ctx context.Context, c *domain.Comment) error {
	m := commentModel{TaskID: c.TaskID, UserID: c.UserID, Content: c.Content}
	if err := r.db.Conn(ctx).Create(&m).Error; err != nil {
		return err
	}
	c.ID, c.CreatedAt = m.ID, m.CreatedAt
	return nil
}

func (r *CommentRepository) List(ctx context.Context, taskID int64, limit, offset int) ([]domain.Comment, error) {
	comments := []domain.Comment{}
	err := r.db.Conn(ctx).Raw(`
		SELECT id, task_id, user_id, content, created_at
		FROM task_comments
		WHERE task_id = ?
		ORDER BY id
		LIMIT ? OFFSET ?`, taskID, limit, offset).Scan(&comments).Error
	return comments, err
}
