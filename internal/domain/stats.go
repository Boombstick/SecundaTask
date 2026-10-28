package domain

import "time"

type StatusCount struct {
	Status TaskStatus
	Count  int64
}

type TopAssignee struct {
	UserID      int64
	Name        string
	ClosedTasks int64
}

type TeamStats struct {
	TeamID          int64
	TasksByStatus   []StatusCount
	TopAssignees30d []TopAssignee
	AvgCloseMinutes *float64
	AvgCloseHours   *float64
	TotalComments   int64
	GeneratedAt     time.Time
}
