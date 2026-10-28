package service

import (
	"context"

	"taskservice/internal/domain"
	"taskservice/internal/platform/apperr"
	"taskservice/internal/platform/validate"
)

type HistoryStore interface {
	Add(ctx context.Context, taskID, changedBy int64, changes domain.Changes) error
	AddBatch(ctx context.Context, taskIDs []int64, changedBy int64, changes domain.Changes) error
	List(ctx context.Context, taskID int64, limit, offset int) ([]domain.HistoryEntry, error)
}

type HistoryService struct {
	history HistoryStore
	tasks   TaskReader
}

func NewHistoryService(history HistoryStore, tasks TaskReader) *HistoryService {
	return &HistoryService{history: history, tasks: tasks}
}

func (s *HistoryService) List(ctx context.Context, userID, taskID int64, q validate.PageQuery) ([]domain.HistoryEntry, error) {
	var v apperr.ValidationErrors
	limit, offset := validate.Page(&v, q)
	if err := v.Err(); err != nil {
		return nil, err
	}
	if _, err := s.tasks.Get(ctx, userID, taskID); err != nil {
		return nil, err
	}
	return s.history.List(ctx, taskID, limit, offset)
}
