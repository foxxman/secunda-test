package auth

import (
	"fmt"
	"strconv"
	"time"

	"task-service/internal/domain"

	"github.com/golang-jwt/jwt/v5"
)

const issuer = "task-service"

type Token struct {
	AccessToken string
	ExpiresAt   time.Time
}

type Manager struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewManager(secret string, ttl time.Duration) *Manager {
	return &Manager{
		secret: []byte(secret),
		ttl:    ttl,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

func (m *Manager) Issue(userID int64) (Token, error) {
	now := m.now()
	expires := now.Add(m.ttl)
	claims := jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(userID, 10),
		Issuer:    issuer,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(expires),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	if err != nil {
		return Token{}, err
	}
	return Token{AccessToken: signed, ExpiresAt: expires}, nil
}

func (m *Manager) Parse(raw string) (int64, error) {
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return m.secret, nil
	})
	if err != nil || !token.Valid {
		return 0, domain.ErrUnauthorized
	}
	if claims.Issuer != issuer {
		return 0, domain.ErrUnauthorized
	}
	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || userID <= 0 {
		return 0, domain.ErrUnauthorized
	}
	return userID, nil
}
