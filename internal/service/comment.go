package service

import (
	"context"
	"strings"

	"taskservice/internal/domain"
	"taskservice/internal/platform/apperr"
	"taskservice/internal/platform/validate"
)

type CommentStore interface {
	Create(ctx context.Context, c *domain.Comment) error
	List(ctx context.Context, taskID int64, limit, offset int) ([]domain.Comment, error)
}

type TaskReader interface {
	Get(ctx context.Context, userID, taskID int64) (domain.Task, error)
}

type CommentService struct {
	comments CommentStore
	tasks    TaskReader
}

func NewCommentService(comments CommentStore, tasks TaskReader) *CommentService {
	return &CommentService{comments: comments, tasks: tasks}
}

func (s *CommentService) Add(ctx context.Context, userID, taskID int64, content string) (domain.Comment, error) {
	content = strings.TrimSpace(content)
	if err := validate.LongText("content", content, 1); err != nil {
		return domain.Comment{}, err
	}
	if _, err := s.tasks.Get(ctx, userID, taskID); err != nil {
		return domain.Comment{}, err
	}
	c := domain.Comment{TaskID: taskID, UserID: userID, Content: content}
	if err := s.comments.Create(ctx, &c); err != nil {
		return domain.Comment{}, err
	}
	return c, nil
}

func (s *CommentService) List(ctx context.Context, userID, taskID int64, q validate.PageQuery) ([]domain.Comment, error) {
	var v apperr.ValidationErrors
	limit, offset := validate.Page(&v, q)
	if err := v.Err(); err != nil {
		return nil, err
	}
	if _, err := s.tasks.Get(ctx, userID, taskID); err != nil {
		return nil, err
	}
	return s.comments.List(ctx, taskID, limit, offset)
}
