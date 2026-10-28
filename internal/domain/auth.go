package domain

import "taskservice/internal/platform/apperr"

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

var ErrInvalidRefreshToken = apperr.Unauthorized("refresh token is invalid or expired")
