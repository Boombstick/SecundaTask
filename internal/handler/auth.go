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

type RegisterRequest struct {
	// 2–256 characters: letters, spaces, hyphens, apostrophes, dots; starts with a letter.
	Name string `json:"name" example:"Анна"`
	// Up to 256 characters; stored lower-cased.
	Email string `json:"email" example:"anna@example.com"`
	// 8+ characters, at most 72 bytes; lowercase, uppercase and a digit; no spaces.
	Password string `json:"password" example:"Password1"`
} //@name RegisterRequest

type LoginRequest struct {
	Email    string `json:"email" example:"anna@example.com"`
	Password string `json:"password" example:"Password1"`
} //@name LoginRequest

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
} //@name RefreshRequest

type TokenPairResponse struct {
	// JWT (HS256): user id in "sub", expiry in "exp".
	AccessToken string `json:"access_token"`
	// Opaque, single-use token for POST /refresh.
	RefreshToken string `json:"refresh_token"`
} //@name TokenPair

type UserResponse struct {
	ID        int64     `json:"id" example:"1"`
	Email     string    `json:"email" example:"anna@example.com"`
	Name      string    `json:"name" example:"Анна"`
	CreatedAt time.Time `json:"created_at"`
} //@name User

func toTokenPair(p domain.TokenPair) TokenPairResponse {
	return TokenPairResponse{AccessToken: p.AccessToken, RefreshToken: p.RefreshToken}
}

func toUser(u domain.User) UserResponse {
	return UserResponse{ID: u.ID, Email: u.Email, Name: u.Name, CreatedAt: u.CreatedAt}
}

type AuthService interface {
	Register(ctx context.Context, email, password, name string) (domain.User, error)
	Login(ctx context.Context, email, password string) (domain.TokenPair, error)
	Refresh(ctx context.Context, refreshToken string) (domain.TokenPair, error)
	Logout(ctx context.Context, refreshToken string) error
}

type AuthHandler struct {
	base
	svc AuthService
}

func NewAuthHandler(svc AuthService, log *slog.Logger) *AuthHandler {
	return &AuthHandler{base: base{log: log}, svc: svc}
}

func (h *AuthHandler) Routes(r chi.Router) {
	r.Post("/register", h.register)
	r.Post("/login", h.login)
	r.Post("/refresh", h.refresh)
	r.Post("/logout", h.logout)
}

// @Summary		Регистрация пользователя
// @Description	Все ошибки валидации приходят одним ответом в `error.details`.
// @Tags			auth
// @Accept			json
// @Produce		json
// @Param			request	body		RegisterRequest	true	"Данные пользователя"
// @Success		201		{object}	UserResponse
// @Failure		400		{object}	httpx.ErrorResponse	"validation_error"
// @Failure		409		{object}	httpx.ErrorResponse	"email_taken"
// @Failure		429		{object}	httpx.ErrorResponse	"rate_limited"
// @Router			/register [post]
func (h *AuthHandler) register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	u, err := h.svc.Register(r.Context(), req.Email, req.Password, req.Name)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toUser(u))
}

// @Summary	Вход: выдача access (JWT) и refresh токенов
// @Tags		auth
// @Accept		json
// @Produce	json
// @Param		request	body		LoginRequest	true	"Учётные данные"
// @Success	200		{object}	TokenPairResponse
// @Failure	400		{object}	httpx.ErrorResponse	"validation_error"
// @Failure	401		{object}	httpx.ErrorResponse	"unauthorized"
// @Failure	429		{object}	httpx.ErrorResponse	"rate_limited"
// @Router		/login [post]
func (h *AuthHandler) login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	pair, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toTokenPair(pair))
}

// @Summary		Обновление пары токенов
// @Description	Предъявленный refresh-токен отзывается, выдаётся новая пара (ротация).
// @Tags			auth
// @Accept			json
// @Produce		json
// @Param			request	body		RefreshRequest	true	"Refresh-токен"
// @Success		200		{object}	TokenPairResponse
// @Failure		401		{object}	httpx.ErrorResponse	"unauthorized"
// @Router			/refresh [post]
func (h *AuthHandler) refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	pair, err := h.svc.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toTokenPair(pair))
}

// @Summary	Отзыв refresh-токена (идемпотентно)
// @Tags		auth
// @Accept		json
// @Param		request	body	RefreshRequest	true	"Refresh-токен"
// @Success	204
// @Failure	400	{object}	httpx.ErrorResponse	"validation_error"
// @Router		/logout [post]
func (h *AuthHandler) logout(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.svc.Logout(r.Context(), req.RefreshToken); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
