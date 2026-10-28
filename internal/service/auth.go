package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"taskservice/internal/domain"
	"taskservice/internal/platform/apperr"
)

type UserStore interface {
	Create(ctx context.Context, u *domain.User) error
	EmailExists(ctx context.Context, email string) (bool, error)
	GetByEmail(ctx context.Context, email string) (domain.User, error)
	GetByID(ctx context.Context, id int64) (domain.User, error)
}

type RefreshTokenStore interface {
	Create(ctx context.Context, userID int64, hash string, expiresAt time.Time) error
	Revoke(ctx context.Context, hash string, now time.Time) (int64, error)
}

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("timing-equalizer"), bcrypt.DefaultCost)

type AuthService struct {
	users      UserStore
	tokens     RefreshTokenStore
	tx         Transactor
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	now        func() time.Time
}

func NewAuthService(users UserStore, tokens RefreshTokenStore, tx Transactor, secret string, accessTTL, refreshTTL time.Duration) *AuthService {
	return &AuthService{
		users: users, tokens: tokens, tx: tx, secret: []byte(secret),
		accessTTL: accessTTL, refreshTTL: refreshTTL,
		now: func() time.Time { return time.Now().UTC() },
	}
}

func (s *AuthService) Register(ctx context.Context, email, password, name string) (domain.User, error) {
	email = NormalizeEmail(email)
	name = strings.TrimSpace(name)
	var v apperr.ValidationErrors
	v.Check(ValidatePersonName(name))
	v.Check(ValidateEmail(email))
	v.Check(ValidatePassword(password))
	if err := v.Err(); err != nil {
		return domain.User{}, err
	}
	if err := s.CheckEmail(ctx, email); err != nil {
		return domain.User{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, err
	}
	u := domain.User{Email: email, PasswordHash: string(hash), Name: name}
	if err := s.users.Create(ctx, &u); err != nil {
		return domain.User{}, err
	}
	return u, nil
}

func (s *AuthService) CheckEmail(ctx context.Context, email string) error {
	exists, err := s.users.EmailExists(ctx, email)
	if err != nil {
		return err
	}
	if exists {
		return domain.ErrEmailTaken
	}
	return nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (domain.TokenPair, error) {
	invalid := apperr.Unauthorized("invalid email or password")
	u, err := s.users.GetByEmail(ctx, NormalizeEmail(email))
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
			return domain.TokenPair{}, invalid
		}
		return domain.TokenPair{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return domain.TokenPair{}, invalid
	}
	return s.issue(ctx, u.ID)
}

func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (domain.TokenPair, error) {
	var pair domain.TokenPair
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		userID, err := s.tokens.Revoke(ctx, hashToken(refreshToken), s.now())
		if err != nil {
			return err
		}
		pair, err = s.issue(ctx, userID)
		return err
	})
	return pair, err
}

func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	_, err := s.tokens.Revoke(ctx, hashToken(refreshToken), s.now())
	if apperr.IsKind(err, apperr.KindUnauthorized) {
		return nil
	}
	return err
}

func (s *AuthService) ParseAccessToken(token string) (int64, error) {
	claims := &jwt.RegisteredClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) { return s.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return 0, apperr.Unauthorized("access token is invalid or expired")
	}
	id, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || id <= 0 {
		return 0, apperr.Unauthorized("access token is invalid")
	}
	return id, nil
}

func (s *AuthService) issue(ctx context.Context, userID int64) (domain.TokenPair, error) {
	now := s.now()
	access, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(userID, 10),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTTL)),
	}).SignedString(s.secret)
	if err != nil {
		return domain.TokenPair{}, err
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return domain.TokenPair{}, err
	}
	refresh := base64.RawURLEncoding.EncodeToString(buf)
	if err := s.tokens.Create(ctx, userID, hashToken(refresh), now.Add(s.refreshTTL)); err != nil {
		return domain.TokenPair{}, err
	}
	return domain.TokenPair{AccessToken: access, RefreshToken: refresh}, nil
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}
