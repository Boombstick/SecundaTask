package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"taskservice/internal/domain"
)

type fakeUsers struct {
	byEmail map[string]domain.User
	creates int
	loads   int
}

func (f *fakeUsers) EmailExists(_ context.Context, email string) (bool, error) {
	_, ok := f.byEmail[email]
	return ok, nil
}

func (f *fakeUsers) Create(_ context.Context, u *domain.User) error {
	f.creates++
	if _, ok := f.byEmail[u.Email]; ok {
		return domain.ErrEmailTaken
	}
	u.ID = int64(len(f.byEmail) + 1)
	f.byEmail[u.Email] = *u
	return nil
}

func (f *fakeUsers) GetByEmail(_ context.Context, email string) (domain.User, error) {
	f.loads++
	if u, ok := f.byEmail[email]; ok {
		return u, nil
	}
	return domain.User{}, domain.ErrUserNotFound
}

func (f *fakeUsers) GetByID(context.Context, int64) (domain.User, error) {
	return domain.User{}, domain.ErrUserNotFound
}

func TestRegister_DuplicateEmailIsRejectedBeforeInsert(t *testing.T) {
	users := &fakeUsers{byEmail: map[string]domain.User{}}
	svc := NewAuthService(users, nil, nil, "secret-secret-secret-secret-secret", time.Hour, time.Hour)
	ctx := context.Background()

	if _, err := svc.Register(ctx, "anna@example.com", "Secret123", "Anna"); err != nil {
		t.Fatalf("first registration: %v", err)
	}

	_, err := svc.Register(ctx, "  ANNA@example.com ", "Secret123", "Anna")
	if !errors.Is(err, domain.ErrEmailTaken) {
		t.Fatalf("want ErrEmailTaken, got %v", err)
	}
	if users.creates != 1 {
		t.Errorf("duplicate must not reach INSERT (it would consume an id): %d inserts", users.creates)
	}
	if users.loads != 0 {
		t.Errorf("the uniqueness check must not load the user: %d loads", users.loads)
	}
}
