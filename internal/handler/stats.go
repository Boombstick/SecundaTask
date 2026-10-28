package handler

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"taskservice/internal/domain"
	"taskservice/internal/platform/httpx"
)

type StatusCountResponse struct {
	Status string `json:"status" enums:"backlog,in_progress,need_info,done" example:"done"`
	Count  int64  `json:"count" example:"12"`
} //@name StatusCount

type TopAssigneeResponse struct {
	UserID      int64  `json:"user_id" example:"2"`
	Name        string `json:"name" example:"Антон"`
	ClosedTasks int64  `json:"closed_tasks" example:"5"`
} //@name TopAssignee

type TeamStatsResponse struct {
	TeamID int64 `json:"team_id" example:"1"`
	// Task count per status, zeros included.
	TasksByStatus []StatusCountResponse `json:"tasks_by_status"`
	// Top 3 assignees by tasks closed in the last 30 days.
	TopAssignees30d []TopAssigneeResponse `json:"top_assignees_30d"`
	// Average time from creation to closing over all closed tasks, minutes.
	AvgCloseMinutes *float64 `json:"avg_close_minutes" example:"7320"`
	// The same value in hours.
	AvgCloseHours *float64 `json:"avg_close_hours" example:"122"`
	// Comments on all tasks of the team.
	TotalComments int64     `json:"total_comments" example:"6"`
	GeneratedAt   time.Time `json:"generated_at"`
} //@name TeamStats

func toTeamStats(r domain.TeamStats) TeamStatsResponse {
	return TeamStatsResponse{
		TeamID: r.TeamID,
		TasksByStatus: httpx.MapSlice(r.TasksByStatus, func(c domain.StatusCount) StatusCountResponse {
			return StatusCountResponse{Status: string(c.Status), Count: c.Count}
		}),
		TopAssignees30d: httpx.MapSlice(r.TopAssignees30d, func(a domain.TopAssignee) TopAssigneeResponse {
			return TopAssigneeResponse{UserID: a.UserID, Name: a.Name, ClosedTasks: a.ClosedTasks}
		}),
		AvgCloseMinutes: r.AvgCloseMinutes,
		AvgCloseHours:   r.AvgCloseHours,
		TotalComments:   r.TotalComments,
		GeneratedAt:     r.GeneratedAt,
	}
}

type StatsService interface {
	TeamStats(ctx context.Context, userID, teamID int64) (domain.TeamStats, error)
}

type StatsHandler struct {
	base
	svc StatsService
}

func NewStatsHandler(svc StatsService, log *slog.Logger) *StatsHandler {
	return &StatsHandler{base: base{log: log}, svc: svc}
}

func (h *StatsHandler) Routes(r chi.Router) {
	r.Get("/teams/{id}/stats", h.teamStats)
}

// @Summary		SQL-отчёт по команде (только owner или admin)
// @Description	Один запрос с CTE: задачи по статусам, топ-3 исполнителя за 30 дней, среднее время закрытия, общее число комментариев к задачам команды. Все подсчёты, округление и порядок — в MySQL.
// @Tags			stats
// @Produce		json
// @Security		BearerAuth
// @Param			id	path		int	true	"ID команды"
// @Success		200	{object}	TeamStatsResponse
// @Failure		403	{object}	httpx.ErrorResponse	"forbidden"
// @Failure		404	{object}	httpx.ErrorResponse	"team_not_found"
// @Failure		401	{object}	httpx.ErrorResponse	"unauthorized"
// @Router			/teams/{id}/stats [get]
func (h *StatsHandler) teamStats(w http.ResponseWriter, r *http.Request) {
	teamID, err := httpx.PathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	rep, err := h.svc.TeamStats(r.Context(), httpx.UserID(r.Context()), teamID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toTeamStats(rep))
}
