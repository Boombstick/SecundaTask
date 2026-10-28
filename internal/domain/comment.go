package domain

import "time"

type Comment struct {
	ID        int64
	TaskID    int64
	UserID    int64
	Content   string
	CreatedAt time.Time
}
