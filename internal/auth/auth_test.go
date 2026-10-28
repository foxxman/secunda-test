package auth

import (
	"strings"
	"testing"
	"time"

	"task-service/internal/domain"

	"github.com/golang-jwt/jwt/v5"
)

func TestPassword(t *testing.T) {
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("expected short password error")
	}
	if _, err := HashPassword(strings.Repeat("a", 73)); err == nil {
		t.Fatal("expected long password error")
	}
	hash, err := HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckPassword(hash, "password123"); err != nil {
		t.Fatal(err)
	}
	if err := CheckPassword(hash, "other-password"); err != domain.ErrUnauthorized {
		t.Fatalf("err = %v", err)
	}
}

func TestToken(t *testing.T) {
	manager := NewManager("test-secret-value", time.Hour)
	issued, err := manager.Issue(42)
	if err != nil {
		t.Fatal(err)
	}
	userID, err := manager.Parse(issued.AccessToken)
	if err != nil || userID != 42 {
		t.Fatalf("user = %d, err = %v", userID, err)
	}

	manager.now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
	expired, err := manager.Issue(7)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Parse(expired.AccessToken); err != domain.ErrUnauthorized {
		t.Fatalf("expired err = %v", err)
	}
	if _, err := manager.Parse("not-a-token"); err != domain.ErrUnauthorized {
		t.Fatalf("garbage err = %v", err)
	}

	other := NewManager("another-secret-value", time.Hour)
	if _, err := other.Parse(issued.AccessToken); err != domain.ErrUnauthorized {
		t.Fatalf("secret err = %v", err)
	}

	claims := jwt.RegisteredClaims{Subject: "1", Issuer: "other", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-secret-value"))
	if err != nil {
		t.Fatal(err)
	}
	manager.now = time.Now
	if _, err := manager.Parse(signed); err != domain.ErrUnauthorized {
		t.Fatalf("issuer err = %v", err)
	}
}
