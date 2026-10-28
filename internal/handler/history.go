package handler

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"taskservice/internal/domain"
	"taskservice/internal/platform/httpx"
	"taskservice/internal/platform/validate"
)

type FieldChangeResponse struct {
	Old any `json:"old"`
	New any `json:"new"`
} //@name FieldChange

type HistoryEntryResponse struct {
	ID        int64 `json:"id" example:"1"`
	TaskID    int64 `json:"task_id" example:"1"`
	ChangedBy int64 `json:"changed_by" example:"1"`
	// Changed fields: {"status": {"old": "backlog", "new": "in_progress"}}.
	Changes   map[string]FieldChangeResponse `json:"changes"`
	CreatedAt time.Time                      `json:"created_at"`
} //@name HistoryEntry

type HistoryListResponse struct {
	Items []HistoryEntryResponse `json:"items"`
} //@name HistoryList

func toHistoryEntry(e domain.HistoryEntry) HistoryEntryResponse {
	changes := make(map[string]FieldChangeResponse, len(e.Changes))
	for field, c := range e.Changes {
		changes[field] = FieldChangeResponse{Old: c.Old, New: c.New}
	}
	return HistoryEntryResponse{ID: e.ID, TaskID: e.TaskID, ChangedBy: e.ChangedBy, Changes: changes, CreatedAt: e.CreatedAt}
}

type HistoryService interface {
	List(ctx context.Context, userID, taskID int64, q validate.PageQuery) ([]domain.HistoryEntry, error)
}

type HistoryHandler struct {
	base
	svc HistoryService
}

func NewHistoryHandler(svc HistoryService, log *slog.Logger) *HistoryHandler {
	return &HistoryHandler{base: base{log: log}, svc: svc}
}

func (h *HistoryHandler) Routes(r chi.Router) {
	r.Get("/tasks/{id}/history", h.list)
}

// @Summary	История изменений задачи
// @Tags		tasks
// @Produce	json
// @Security	BearerAuth
// @Param		id		path		int	true	"ID задачи"
// @Param		limit	query		int	false	"1–100"		default(20)
// @Param		offset	query		int	false	"Смещение"	default(0)
// @Success	200		{object}	HistoryListResponse
// @Failure	400		{object}	httpx.ErrorResponse	"validation_error"
// @Failure	404		{object}	httpx.ErrorResponse	"task_not_found"
// @Failure	401		{object}	httpx.ErrorResponse	"unauthorized"
// @Router		/tasks/{id}/history [get]
func (h *HistoryHandler) list(w http.ResponseWriter, r *http.Request) {
	taskID, err := httpx.PathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	q := r.URL.Query()
	page := validate.PageQuery{Limit: q.Get("limit"), Offset: q.Get("offset")}
	items, err := h.svc.List(r.Context(), httpx.UserID(r.Context()), taskID, page)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, HistoryListResponse{Items: httpx.MapSlice(items, toHistoryEntry)})
}
