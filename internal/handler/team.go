package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"taskservice/internal/domain"
	"taskservice/internal/platform/httpx"
)

type CreateTeamRequest struct {
	// 1–256 characters, single line.
	Name string `json:"name" example:"Backend"`
} //@name CreateTeamRequest

type InviteRequest struct {
	UserID int64 `json:"user_id" example:"2"`
	// Owner may grant admin or member, admin — member only. Default: member.
	Role string `json:"role" enums:"admin,member" example:"member"`
} //@name InviteRequest

type ChangeRoleRequest struct {
	Role string `json:"role" enums:"admin,member" example:"admin"`
} //@name ChangeRoleRequest

type TeamResponse struct {
	ID   int64  `json:"id" example:"1"`
	Name string `json:"name" example:"Backend"`
	// Role of the current user in the team.
	Role string `json:"role" enums:"owner,admin,member" example:"owner"`
} //@name Team

type TeamListResponse struct {
	Items []TeamResponse `json:"items"`
} //@name TeamList

type MemberResponse struct {
	TeamID int64  `json:"team_id" example:"1"`
	UserID int64  `json:"user_id" example:"2"`
	Role   string `json:"role" enums:"owner,admin,member" example:"member"`
	Email  string `json:"email" example:"anna@example.com"`
	Name   string `json:"name" example:"Анна-Мария"`
} //@name Member

type MemberListResponse struct {
	Items []MemberResponse `json:"items"`
} //@name MemberList

func toTeam(t domain.Team) TeamResponse {
	return TeamResponse{ID: t.ID, Name: t.Name, Role: string(t.Role)}
}

func toMember(m domain.Member) MemberResponse {
	return MemberResponse{TeamID: m.TeamID, UserID: m.UserID, Role: string(m.Role), Email: m.Email, Name: m.Name}
}

type TeamService interface {
	Create(ctx context.Context, userID int64, name string) (domain.Team, error)
	List(ctx context.Context, userID int64) ([]domain.Team, error)
	Members(ctx context.Context, userID, teamID int64) ([]domain.Member, error)
	Invite(ctx context.Context, userID, teamID, inviteeID int64, role domain.Role) (domain.Member, error)
	ChangeRole(ctx context.Context, userID, teamID, targetID int64, role domain.Role) (domain.Member, error)
	RemoveMember(ctx context.Context, userID, teamID, targetID int64) error
	Delete(ctx context.Context, userID, teamID int64) error
}

type TeamHandler struct {
	base
	svc TeamService
}

func NewTeamHandler(svc TeamService, log *slog.Logger) *TeamHandler {
	return &TeamHandler{base: base{log: log}, svc: svc}
}

func (h *TeamHandler) Routes(r chi.Router) {
	r.Post("/teams", h.create)
	r.Get("/teams", h.list)
	r.Delete("/teams/{id}", h.delete)
	r.Post("/teams/{id}/invite", h.invite)
	r.Get("/teams/{id}/members", h.members)
	r.Patch("/teams/{id}/members/{user_id}", h.changeRole)
	r.Delete("/teams/{id}/members/{user_id}", h.removeMember)
}

// @Summary	Создание команды (текущий пользователь становится owner)
// @Tags		teams
// @Accept		json
// @Produce	json
// @Security	BearerAuth
// @Param		request	body		CreateTeamRequest	true	"Команда"
// @Success	201		{object}	TeamResponse
// @Failure	400		{object}	httpx.ErrorResponse	"validation_error"
// @Failure	401		{object}	httpx.ErrorResponse	"unauthorized"
// @Router		/teams [post]
func (h *TeamHandler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateTeamRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	t, err := h.svc.Create(r.Context(), httpx.UserID(r.Context()), req.Name)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toTeam(t))
}

// @Summary	Команды текущего пользователя
// @Tags		teams
// @Produce	json
// @Security	BearerAuth
// @Success	200	{object}	TeamListResponse
// @Failure	401	{object}	httpx.ErrorResponse	"unauthorized"
// @Router		/teams [get]
func (h *TeamHandler) list(w http.ResponseWriter, r *http.Request) {
	teams, err := h.svc.List(r.Context(), httpx.UserID(r.Context()))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, TeamListResponse{Items: httpx.MapSlice(teams, toTeam)})
}

// @Summary		Удаление команды (только owner)
// @Description	Задачи, история, комментарии и участники удаляются каскадно.
// @Tags			teams
// @Security		BearerAuth
// @Param			id	path	int	true	"ID команды"
// @Success		204
// @Failure		403	{object}	httpx.ErrorResponse	"forbidden"
// @Failure		404	{object}	httpx.ErrorResponse	"team_not_found"
// @Failure		401	{object}	httpx.ErrorResponse	"unauthorized"
// @Router			/teams/{id} [delete]
func (h *TeamHandler) delete(w http.ResponseWriter, r *http.Request) {
	teamID, err := httpx.PathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.svc.Delete(r.Context(), httpx.UserID(r.Context()), teamID); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary		Добавление пользователя в команду
// @Description	Owner добавляет с ролью admin или member, admin — только member. Роль owner не выдаётся.
// @Tags			teams
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			id		path		int				true	"ID команды"
// @Param			request	body		InviteRequest	true	"Кого и с какой ролью"
// @Success		201		{object}	MemberResponse
// @Failure		400		{object}	httpx.ErrorResponse	"validation_error"
// @Failure		403		{object}	httpx.ErrorResponse	"forbidden"
// @Failure		404		{object}	httpx.ErrorResponse	"team_not_found, user_not_found"
// @Failure		409		{object}	httpx.ErrorResponse	"already_member"
// @Failure		401		{object}	httpx.ErrorResponse	"unauthorized"
// @Router			/teams/{id}/invite [post]
func (h *TeamHandler) invite(w http.ResponseWriter, r *http.Request) {
	teamID, err := httpx.PathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var req InviteRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	m, err := h.svc.Invite(r.Context(), httpx.UserID(r.Context()), teamID, req.UserID, domain.Role(req.Role))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toMember(m))
}

// @Summary	Участники команды (любому участнику)
// @Tags		teams
// @Produce	json
// @Security	BearerAuth
// @Param		id	path		int	true	"ID команды"
// @Success	200	{object}	MemberListResponse
// @Failure	404	{object}	httpx.ErrorResponse	"team_not_found"
// @Failure	401	{object}	httpx.ErrorResponse	"unauthorized"
// @Router		/teams/{id}/members [get]
func (h *TeamHandler) members(w http.ResponseWriter, r *http.Request) {
	teamID, err := httpx.PathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	members, err := h.svc.Members(r.Context(), httpx.UserID(r.Context()), teamID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, MemberListResponse{Items: httpx.MapSlice(members, toMember)})
}

// @Summary	Смена роли участника (только owner)
// @Tags		teams
// @Accept		json
// @Produce	json
// @Security	BearerAuth
// @Param		id		path		int					true	"ID команды"
// @Param		user_id	path		int					true	"ID участника"
// @Param		request	body		ChangeRoleRequest	true	"Новая роль"
// @Success	200		{object}	MemberResponse
// @Failure	400		{object}	httpx.ErrorResponse	"validation_error"
// @Failure	403		{object}	httpx.ErrorResponse	"forbidden"
// @Failure	404		{object}	httpx.ErrorResponse	"team_not_found, member_not_found"
// @Failure	401		{object}	httpx.ErrorResponse	"unauthorized"
// @Router		/teams/{id}/members/{user_id} [patch]
func (h *TeamHandler) changeRole(w http.ResponseWriter, r *http.Request) {
	teamID, err := httpx.PathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	targetID, err := httpx.PathID(r, "user_id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var req ChangeRoleRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	m, err := h.svc.ChangeRole(r.Context(), httpx.UserID(r.Context()), teamID, targetID, domain.Role(req.Role))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toMember(m))
}

// @Summary		Удаление участника из команды
// @Description	Owner удаляет любого, кроме себя; admin — только member; любой не-owner может выйти сам. Удалённый снимается с задач команды (с записью в историю).
// @Tags			teams
// @Security		BearerAuth
// @Param			id		path	int	true	"ID команды"
// @Param			user_id	path	int	true	"ID участника"
// @Success		204
// @Failure		403	{object}	httpx.ErrorResponse	"forbidden"
// @Failure		404	{object}	httpx.ErrorResponse	"team_not_found, member_not_found"
// @Failure		401	{object}	httpx.ErrorResponse	"unauthorized"
// @Router			/teams/{id}/members/{user_id} [delete]
func (h *TeamHandler) removeMember(w http.ResponseWriter, r *http.Request) {
	teamID, err := httpx.PathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	targetID, err := httpx.PathID(r, "user_id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.svc.RemoveMember(r.Context(), httpx.UserID(r.Context()), teamID, targetID); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
