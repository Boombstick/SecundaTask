package repository

import (
	"context"
	"fmt"
	"time"

	"taskservice/internal/domain"
	"taskservice/internal/platform/database"
)

const teamStatsQuery = `
WITH team_tasks AS (
    SELECT id, status, assignee_id, created_at, closed_at
    FROM tasks
    WHERE team_id = @team
),
totals AS (
    SELECT
        COALESCE(SUM(status = 'backlog'), 0)     AS backlog,
        COALESCE(SUM(status = 'in_progress'), 0) AS in_progress,
        COALESCE(SUM(status = 'need_info'), 0)   AS need_info,
        COALESCE(SUM(status = 'done'), 0)        AS done,
        ROUND(AVG(TIMESTAMPDIFF(SECOND, created_at, closed_at)) / 60, 2)   AS avg_close_minutes,
        ROUND(AVG(TIMESTAMPDIFF(SECOND, created_at, closed_at)) / 3600, 2) AS avg_close_hours
    FROM team_tasks
),
comments AS (
    SELECT COUNT(*) AS total_comments
    FROM task_comments c
    JOIN team_tasks t ON t.id = c.task_id
),
top_assignees AS (
    SELECT u.id AS user_id, u.name, COUNT(*) AS closed_tasks,
           ROW_NUMBER() OVER (ORDER BY COUNT(*) DESC, u.id) AS rn
    FROM team_tasks t
    JOIN users u ON u.id = t.assignee_id
    WHERE t.status = 'done'
      AND t.closed_at >= @now - INTERVAL 30 DAY
      AND t.closed_at <= @now
    GROUP BY u.id, u.name
)
SELECT tot.backlog, tot.in_progress, tot.need_info, tot.done,
       tot.avg_close_minutes, tot.avg_close_hours,
       cm.total_comments,
       ta.user_id, ta.name, ta.closed_tasks
FROM totals tot
CROSS JOIN comments cm
LEFT JOIN top_assignees ta ON ta.rn <= 3
ORDER BY ta.rn
`

type StatsRepository struct {
	db *database.DB
}

func NewStatsRepository(db *database.DB) *StatsRepository { return &StatsRepository{db: db} }

func (r *StatsRepository) TeamStats(ctx context.Context, teamID int64, now time.Time) (domain.TeamStats, error) {
	var rows []struct {
		Backlog         int64
		InProgress      int64
		NeedInfo        int64
		Done            int64
		AvgCloseMinutes *float64
		AvgCloseHours   *float64
		TotalComments   int64
		UserID          *int64
		Name            *string
		ClosedTasks     *int64
	}
	err := r.db.Conn(ctx).Raw(teamStatsQuery, map[string]any{
		"team": teamID, "now": now,
	}).Scan(&rows).Error
	if err != nil {
		return domain.TeamStats{}, fmt.Errorf("team stats query: %w", err)
	}
	if len(rows) == 0 {
		return domain.TeamStats{}, fmt.Errorf("team stats query returned no rows")
	}

	head := rows[0]
	st := domain.TeamStats{
		TeamID: teamID,
		TasksByStatus: []domain.StatusCount{
			{Status: domain.StatusBacklog, Count: head.Backlog},
			{Status: domain.StatusInProgress, Count: head.InProgress},
			{Status: domain.StatusNeedInfo, Count: head.NeedInfo},
			{Status: domain.StatusDone, Count: head.Done},
		},
		TopAssignees30d: []domain.TopAssignee{},
		AvgCloseMinutes: head.AvgCloseMinutes,
		AvgCloseHours:   head.AvgCloseHours,
		TotalComments:   head.TotalComments,
		GeneratedAt:     now,
	}
	for _, row := range rows {
		if row.UserID == nil {
			continue
		}
		st.TopAssignees30d = append(st.TopAssignees30d, domain.TopAssignee{
			UserID: *row.UserID, Name: *row.Name, ClosedTasks: *row.ClosedTasks,
		})
	}
	return st, nil
}
