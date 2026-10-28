package repository

import (
	"context"
	"time"

	"taskservice/internal/domain"
	"taskservice/internal/platform/database"
)

type userModel struct {
	ID           int64 `gorm:"primaryKey"`
	Email        string
	PasswordHash string
	Name         string
	CreatedAt    time.Time
}

func (userModel) TableName() string { return "users" }

type UserRepository struct {
	db *database.DB
}

func NewUserRepository(db *database.DB) *UserRepository { return &UserRepository{db: db} }

func (r *UserRepository) Create(ctx context.Context, u *domain.User) error {
	m := userModel{Email: u.Email, PasswordHash: u.PasswordHash, Name: u.Name}
	if err := r.db.Conn(ctx).Create(&m).Error; err != nil {
		if database.IsDuplicate(err) {
			return domain.ErrEmailTaken
		}
		return err
	}
	u.ID, u.CreatedAt = m.ID, m.CreatedAt
	return nil
}

func (r *UserRepository) EmailExists(ctx context.Context, email string) (bool, error) {
	var exists bool
	err := r.db.Conn(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM users WHERE email = ?)`, email).Scan(&exists).Error
	return exists, err
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	var u domain.User
	err := r.db.ScanOne(ctx, &u, domain.ErrUserNotFound,
		`SELECT id, email, password_hash, name, created_at FROM users WHERE email = ?`, email)
	return u, err
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (domain.User, error) {
	var u domain.User
	err := r.db.ScanOne(ctx, &u, domain.ErrUserNotFound,
		`SELECT id, email, password_hash, name, created_at FROM users WHERE id = ?`, id)
	return u, err
}
