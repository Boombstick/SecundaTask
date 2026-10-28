package repository

import (
	"context"
	"time"

	"taskservice/internal/domain"
	"taskservice/internal/platform/database"
)

type refreshTokenModel struct {
	ID        int64 `gorm:"primaryKey"`
	UserID    int64
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

func (refreshTokenModel) TableName() string { return "refresh_tokens" }

type RefreshTokenRepository struct {
	db *database.DB
}

func NewRefreshTokenRepository(db *database.DB) *RefreshTokenRepository {
	return &RefreshTokenRepository{db: db}
}

func (r *RefreshTokenRepository) Create(ctx context.Context, userID int64, hash string, expiresAt time.Time) error {
	return r.db.Conn(ctx).Create(&refreshTokenModel{UserID: userID, TokenHash: hash, ExpiresAt: expiresAt}).Error
}

func (r *RefreshTokenRepository) Revoke(ctx context.Context, hash string, now time.Time) (int64, error) {
	invalid := domain.ErrInvalidRefreshToken
	var tok struct{ UserID int64 }
	if err := r.db.ScanOne(ctx, &tok, invalid,
		`SELECT user_id FROM refresh_tokens WHERE token_hash = ? AND revoked_at IS NULL AND expires_at > ?`,
		hash, now); err != nil {
		return 0, err
	}
	res := r.db.Conn(ctx).Model(&refreshTokenModel{}).
		Where("token_hash = ? AND revoked_at IS NULL", hash).
		Update("revoked_at", now)
	if res.Error != nil {
		return 0, res.Error
	}
	if res.RowsAffected == 0 {
		return 0, invalid
	}
	return tok.UserID, nil
}
