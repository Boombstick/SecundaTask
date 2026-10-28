package domain

import "time"

type FieldChange struct {
	Old any
	New any
}

type Changes map[string]FieldChange

type HistoryEntry struct {
	ID        int64
	TaskID    int64
	ChangedBy int64
	Changes   Changes
	CreatedAt time.Time
}
