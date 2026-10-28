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

type CreateCommentRequest struct {
	// 1–512 characters; line breaks allowed.
	Content string `json:"content" example:"Взяла в работу"`
} //@name CreateCommentRequest

type CommentResponse struct {
	ID        int64     `json:"id" example:"1"`
	TaskID    int64     `json:"task_id" example:"1"`
	UserID    int64     `json:"user_id" example:"3"`
	Content   string    `json:"content" example:"Взяла в работу"`
	CreatedAt time.Time `json:"created_at"`
} //@name Comment

type CommentListResponse struct {
	Items []CommentResponse `json:"items"`
} //@name CommentList

func toComment(c domain.Comment) CommentResponse {
	return CommentResponse{ID: c.ID, TaskID: c.TaskID, UserID: c.UserID, Content: c.Content, CreatedAt: c.CreatedAt}
}

type CommentService interface {
	Add(ctx context.Context, userID, taskID int64, content string) (domain.Comment, error)
	List(ctx context.Context, userID, taskID int64, q validate.PageQuery) ([]domain.Comment, error)
}

type CommentHandler struct {
	base
	svc CommentService
}

func NewCommentHandler(svc CommentService, log *slog.Logger) *CommentHandler {
	return &CommentHandler{base: base{log: log}, svc: svc}
}

func (h *CommentHandler) Routes(r chi.Router) {
	r.Post("/tasks/{id}/comments", h.add)
	r.Get("/tasks/{id}/comments", h.list)
}

// @Summary	Добавление комментария (любой участник команды задачи)
// @Tags		comments
// @Accept		json
// @Produce	json
// @Security	BearerAuth
// @Param		id		path		int						true	"ID задачи"
// @Param		request	body		CreateCommentRequest	true	"Комментарий"
// @Success	201		{object}	CommentResponse
// @Failure	400		{object}	httpx.ErrorResponse	"validation_error"
// @Failure	404		{object}	httpx.ErrorResponse	"task_not_found"
// @Failure	401		{object}	httpx.ErrorResponse	"unauthorized"
// @Router		/tasks/{id}/comments [post]
func (h *CommentHandler) add(w http.ResponseWriter, r *http.Request) {
	taskID, err := httpx.PathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var req CreateCommentRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	c, err := h.svc.Add(r.Context(), httpx.UserID(r.Context()), taskID, req.Content)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toComment(c))
}

// @Summary	Комментарии задачи (от старых к новым)
// @Tags		comments
// @Produce	json
// @Security	BearerAuth
// @Param		id		path		int	true	"ID задачи"
// @Param		limit	query		int	false	"1–100"		default(20)
// @Param		offset	query		int	false	"Смещение"	default(0)
// @Success	200		{object}	CommentListResponse
// @Failure	400		{object}	httpx.ErrorResponse	"validation_error"
// @Failure	404		{object}	httpx.ErrorResponse	"task_not_found"
// @Failure	401		{object}	httpx.ErrorResponse	"unauthorized"
// @Router		/tasks/{id}/comments [get]
func (h *CommentHandler) list(w http.ResponseWriter, r *http.Request) {
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
	httpx.WriteJSON(w, http.StatusOK, CommentListResponse{Items: httpx.MapSlice(items, toComment)})
}
