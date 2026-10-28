package service

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"task-service/internal/auth"
	"task-service/internal/domain"
	"task-service/internal/repository"
)

type Auth struct {
	store  repository.Store
	tokens *auth.Manager
	now    func() time.Time
}

func NewAuth(store repository.Store, tokens *auth.Manager) *Auth {
	return &Auth{
		store:  store,
		tokens: tokens,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

type RegisteredUser struct {
	ID        int64
	Email     string
	Name      string
	CreatedAt time.Time
}

func (a *Auth) Register(ctx context.Context, email, password, name string) (RegisteredUser, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return RegisteredUser{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return RegisteredUser{}, domain.Invalid("имя обязательно")
	}
	if len(name) > 255 {
		return RegisteredUser{}, domain.Invalid("имя слишком длинное")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return RegisteredUser{}, err
	}
	user, err := a.store.CreateUser(ctx, domain.User{
		Email:        email,
		PasswordHash: hash,
		Name:         name,
		CreatedAt:    a.now(),
	})
	if err != nil {
		return RegisteredUser{}, err
	}
	return RegisteredUser{ID: user.ID, Email: user.Email, Name: user.Name, CreatedAt: user.CreatedAt}, nil
}

type Session struct {
	AccessToken string
	ExpiresAt   time.Time
}

func (a *Auth) Login(ctx context.Context, email, password string) (Session, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return Session{}, domain.ErrUnauthorized
	}
	user, err := a.store.UserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return Session{}, domain.ErrUnauthorized
		}
		return Session{}, err
	}
	if err := auth.CheckPassword(user.PasswordHash, password); err != nil {
		return Session{}, domain.ErrUnauthorized
	}
	token, err := a.tokens.Issue(user.ID)
	if err != nil {
		return Session{}, err
	}
	return Session{AccessToken: token.AccessToken, ExpiresAt: token.ExpiresAt}, nil
}

func normalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || !strings.Contains(email, ".") {
		return "", domain.Invalid("некорректный email")
	}
	return email, nil
}
