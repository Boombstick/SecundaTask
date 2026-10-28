package domain

import (
	"time"

	"taskservice/internal/platform/apperr"
)

type User struct {
	ID           int64
	Email        string
	PasswordHash string
	Name         string
	CreatedAt    time.Time
}

var (
	ErrUserNotFound = apperr.NotFound("user_not_found", "user not found")
	ErrEmailTaken   = apperr.Conflict("email_taken", "a user with this email already exists")
)
