package repository

import (
	"context"
	"encoding/json"
	"time"

	"taskservice/internal/domain"
	"taskservice/internal/platform/database"
)

type historyModel struct {
	ID        int64 `gorm:"primaryKey"`
	TaskID    int64
	ChangedBy int64
	Changes   string
	CreatedAt time.Time
}

func (historyModel) TableName() string { return "task_history" }

type storedChange struct {
	Old any `json:"old"`
	New any `json:"new"`
}

func encodeChanges(ch domain.Changes) (string, error) {
	out := make(map[string]storedChange, len(ch))
	for field, c := range ch {
		out[field] = storedChange{Old: c.Old, New: c.New}
	}
	raw, err := json.Marshal(out)
	return string(raw), err
}

func decodeChanges(raw string) (domain.Changes, error) {
	var in map[string]storedChange
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return nil, err
	}
	out := make(domain.Changes, len(in))
	for field, c := range in {
		out[field] = domain.FieldChange{Old: c.Old, New: c.New}
	}
	return out, nil
}

type HistoryRepository struct {
	db *database.DB
}

func NewHistoryRepository(db *database.DB) *HistoryRepository { return &HistoryRepository{db: db} }

func (r *HistoryRepository) Add(ctx context.Context, taskID, changedBy int64, changes domain.Changes) error {
	return r.AddBatch(ctx, []int64{taskID}, changedBy, changes)
}

func (r *HistoryRepository) AddBatch(ctx context.Context, taskIDs []int64, changedBy int64, changes domain.Changes) error {
	if len(taskIDs) == 0 {
		return nil
	}
	raw, err := encodeChanges(changes)
	if err != nil {
		return err
	}
	rows := make([]historyModel, len(taskIDs))
	for i, id := range taskIDs {
		rows[i] = historyModel{TaskID: id, ChangedBy: changedBy, Changes: raw}
	}
	return r.db.Conn(ctx).Create(&rows).Error
}

func (r *HistoryRepository) List(ctx context.Context, taskID int64, limit, offset int) ([]domain.HistoryEntry, error) {
	var rows []struct {
		ID        int64
		TaskID    int64
		ChangedBy int64
		Changes   string
		CreatedAt time.Time
	}
	err := r.db.Conn(ctx).Raw(`
		SELECT id, task_id, changed_by, changes, created_at
		FROM task_history
		WHERE task_id = ?
		ORDER BY id
		LIMIT ? OFFSET ?`, taskID, limit, offset).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.HistoryEntry, len(rows))
	for i, row := range rows {
		changes, err := decodeChanges(row.Changes)
		if err != nil {
			return nil, err
		}
		out[i] = domain.HistoryEntry{
			ID: row.ID, TaskID: row.TaskID, ChangedBy: row.ChangedBy,
			Changes: changes, CreatedAt: row.CreatedAt,
		}
	}
	return out, nil
}
