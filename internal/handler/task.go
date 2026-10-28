package handler

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"taskservice/internal/domain"
	"taskservice/internal/platform/apperr"
	"taskservice/internal/platform/httpx"
	"taskservice/internal/service"
)

type CreateTaskRequest struct {
	TeamID int64 `json:"team_id" example:"1"`
	// 1–256 characters, single line.
	Title string `json:"title" example:"Настроить CI"`
	// Up to 512 characters; line breaks allowed.
	Description string `json:"description" example:"GitHub Actions"`
	AssigneeID  *int64 `json:"assignee_id" example:"2"`
} //@name CreateTaskRequest

type UpdateTaskRequest struct {
	Title       *string `json:"title" example:"Настроить CI"`
	Description string  `json:"description" example:"GitHub Actions"`
	Status      *string `json:"status" enums:"backlog,in_progress,need_info,done" example:"in_progress"`
	AssigneeID  *int64  `json:"assignee_id" example:"2"`
	// Version of the task the client based its edit on.
	Version *int `json:"version" example:"1"`
} //@name UpdateTaskRequest

type EntityResponse struct {
	ID   int64  `json:"id" example:"1"`
	Name string `json:"name" example:"Backend"`
} //@name Entity

type TaskResponse struct {
	ID          int64           `json:"id" example:"1"`
	Team        EntityResponse  `json:"team"`
	Title       string          `json:"title" example:"Настроить CI"`
	Description string          `json:"description" example:"GitHub Actions"`
	Status      string          `json:"status" enums:"backlog,in_progress,need_info,done" example:"in_progress"`
	CreatedBy   EntityResponse  `json:"created_by"`
	Assignee    *EntityResponse `json:"assignee"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	ClosedAt    *time.Time      `json:"closed_at"`
	Version     int             `json:"version" example:"2"`
} //@name Task

type TaskListItem struct {
	ID         int64  `json:"id" example:"1"`
	TeamID     int64  `json:"team_id" example:"1"`
	Title      string `json:"title" example:"Настроить CI"`
	Status     string `json:"status" enums:"backlog,in_progress,need_info,done" example:"in_progress"`
	CreatedBy  int64  `json:"created_by" example:"1"`
	AssigneeID *int64 `json:"assignee_id" example:"2"`
} //@name TaskListItem

type TaskPageResponse struct {
	Items []TaskListItem `json:"items"`
	Limit int            `json:"limit" example:"20"`
	// Offset mode only.
	Offset *int `json:"offset,omitempty" example:"0"`
	// Number of tasks matching the filter; offset mode only.
	Total *int64 `json:"total,omitempty" example:"27"`
	// Cursor of the next page; null when there are no more pages.
	NextCursor *string `json:"next_cursor" example:"aWQ6Mjg"`
} //@name TaskPage

func toEntity(e domain.Entity) EntityResponse {
	return EntityResponse{ID: e.ID, Name: e.Name}
}

func toTask(c domain.TaskCard) TaskResponse {
	resp := TaskResponse{
		ID: c.ID, Team: toEntity(c.Team), Title: c.Title, Description: c.Description,
		Status: string(c.Status), CreatedBy: toEntity(c.Creator),
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, ClosedAt: c.ClosedAt, Version: c.Version,
	}
	if c.Assignee != nil {
		a := toEntity(*c.Assignee)
		resp.Assignee = &a
	}
	return resp
}

func toTaskListItem(t domain.Task) TaskListItem {
	return TaskListItem{ID: t.ID, TeamID: t.TeamID, Title: t.Title, Status: string(t.Status), CreatedBy: t.CreatedBy, AssigneeID: t.AssigneeID}
}

func toTaskPage(p domain.TaskPage) TaskPageResponse {
	return TaskPageResponse{
		Items:      httpx.MapSlice(p.Items, toTaskListItem),
		Limit:      p.Limit,
		Offset:     p.Offset,
		Total:      p.Total,
		NextCursor: p.NextCursor,
	}
}

func optionalStatus(s *string) *domain.TaskStatus {
	if s == nil {
		return nil
	}
	st := domain.TaskStatus(*s)
	return &st
}

type TaskService interface {
	Create(ctx context.Context, userID int64, in service.CreateTaskInput) (domain.TaskCard, error)
	Card(ctx context.Context, userID, taskID int64) (domain.TaskCard, error)
	Update(ctx context.Context, userID, taskID int64, in service.UpdateTaskInput) (domain.TaskCard, error)
	Delete(ctx context.Context, userID, taskID int64, version *int) error
	List(ctx context.Context, userID int64, in service.ListTasksInput) (domain.TaskPage, error)
}

type TaskHandler struct {
	base
	svc TaskService
}

func NewTaskHandler(svc TaskService, log *slog.Logger) *TaskHandler {
	return &TaskHandler{base: base{log: log}, svc: svc}
}

func (h *TaskHandler) Routes(r chi.Router) {
	r.Post("/tasks", h.create)
	r.Get("/tasks", h.list)
	r.Get("/tasks/{id}", h.get)
	r.Put("/tasks/{id}", h.update)
	r.Delete("/tasks/{id}", h.delete)
}

// @Summary		Создание задачи в команде
// @Description	Создать задачу может любой участник команды; он же становится её создателем. Статус назначает сервер (`backlog`); поле `status` в запросе отклоняется.
// @Tags			tasks
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			request	body		CreateTaskRequest	true	"Задача"
// @Success		201		{object}	TaskResponse
// @Failure		400		{object}	httpx.ErrorResponse	"validation_error (все ошибки в details)"
// @Failure		404		{object}	httpx.ErrorResponse	"team_not_found"
// @Failure		401		{object}	httpx.ErrorResponse	"unauthorized"
// @Router			/tasks [post]
func (h *TaskHandler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateTaskRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	t, err := h.svc.Create(r.Context(), httpx.UserID(r.Context()), service.CreateTaskInput{
		TeamID: req.TeamID, Title: req.Title, Description: req.Description, AssigneeID: req.AssigneeID,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toTask(t))
}

// @Summary		Список задач команды (кешируется в Redis на 5 минут)
// @Description	Два режима пагинации: limit+offset (в ответе total и offset) или limit+cursor (next_cursor из предыдущего ответа). Если передан cursor, offset игнорируется. Сортировка от новых к старым.
// @Tags			tasks
// @Produce		json
// @Security		BearerAuth
// @Param			team_id		query		int		true	"ID команды"
// @Param			status		query		string	false	"Статус"	Enums(backlog, in_progress, need_info, done)
// @Param			assignee_id	query		int		false	"ID исполнителя"
// @Param			limit		query		int		false	"Размер страницы, 1–100"	default(20)
// @Param			offset		query		int		false	"Смещение (offset-режим)"	default(0)
// @Param			cursor		query		string	false	"Курсор (cursor-режим)"
// @Success		200			{object}	TaskPageResponse
// @Failure		400			{object}	httpx.ErrorResponse	"validation_error (все ошибки в details)"
// @Failure		404			{object}	httpx.ErrorResponse	"team_not_found"
// @Failure		401			{object}	httpx.ErrorResponse	"unauthorized"
// @Router			/tasks [get]
func (h *TaskHandler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, err := h.svc.List(r.Context(), httpx.UserID(r.Context()), service.ListTasksInput{
		TeamID:     q.Get("team_id"),
		Status:     q.Get("status"),
		AssigneeID: q.Get("assignee_id"),
		Limit:      q.Get("limit"),
		Offset:     q.Get("offset"),
		Cursor:     q.Get("cursor"),
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toTaskPage(page))
}

// @Summary	Задача по id (полная, с version для PUT)
// @Tags		tasks
// @Produce	json
// @Security	BearerAuth
// @Param		id	path		int	true	"ID задачи"
// @Success	200	{object}	TaskResponse
// @Failure	404	{object}	httpx.ErrorResponse	"task_not_found"
// @Failure	401	{object}	httpx.ErrorResponse	"unauthorized"
// @Router		/tasks/{id} [get]
func (h *TaskHandler) get(w http.ResponseWriter, r *http.Request) {
	taskID, err := httpx.PathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	t, err := h.svc.Card(r.Context(), httpx.UserID(r.Context()), taskID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toTask(t))
}

// @Summary		Полное обновление задачи
// @Description	Обязательны title, status и version; не переданное description станет пустым, не переданный assignee_id снимет исполнителя. Owner/admin и создатель меняют всё, исполнитель — только статус. Порядок проверок: 404 → 400 (все ошибки) → 403 → 409. Изменение пишется в историю в той же транзакции.
// @Tags			tasks
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			id		path		int					true	"ID задачи"
// @Param			request	body		UpdateTaskRequest	true	"Новое состояние задачи"
// @Success		200		{object}	TaskResponse
// @Failure		400		{object}	httpx.ErrorResponse	"validation_error (все ошибки в details)"
// @Failure		403		{object}	httpx.ErrorResponse	"forbidden"
// @Failure		404		{object}	httpx.ErrorResponse	"task_not_found"
// @Failure		409		{object}	httpx.ErrorResponse	"version_conflict"
// @Failure		401		{object}	httpx.ErrorResponse	"unauthorized"
// @Router			/tasks/{id} [put]
func (h *TaskHandler) update(w http.ResponseWriter, r *http.Request) {
	taskID, err := httpx.PathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var req UpdateTaskRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	t, err := h.svc.Update(r.Context(), httpx.UserID(r.Context()), taskID, service.UpdateTaskInput{
		Title: req.Title, Description: req.Description, Status: optionalStatus(req.Status),
		AssigneeID: req.AssigneeID, Version: req.Version,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toTask(t))
}

// @Summary		Удаление задачи (owner, admin или создатель)
// @Description	История и комментарии удаляются каскадно.
// @Tags			tasks
// @Security		BearerAuth
// @Param			id		path	int	true	"ID задачи"
// @Param			version	query	int	false	"Удалить только при совпадении версии"
// @Success		204
// @Failure		403	{object}	httpx.ErrorResponse	"forbidden"
// @Failure		404	{object}	httpx.ErrorResponse	"task_not_found"
// @Failure		409	{object}	httpx.ErrorResponse	"version_conflict"
// @Failure		401	{object}	httpx.ErrorResponse	"unauthorized"
// @Router			/tasks/{id} [delete]
func (h *TaskHandler) delete(w http.ResponseWriter, r *http.Request) {
	taskID, err := httpx.PathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var version *int
	if v := r.URL.Query().Get("version"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			h.fail(w, r, apperr.ValidationField("version", "version must be a positive integer"))
			return
		}
		version = &n
	}
	if err := h.svc.Delete(r.Context(), httpx.UserID(r.Context()), taskID, version); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
