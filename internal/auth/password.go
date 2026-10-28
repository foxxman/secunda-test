package auth

import (
	"task-service/internal/domain"

	"golang.org/x/crypto/bcrypt"
)

const (
	minPasswordLen = 8
	maxPasswordLen = 72
)

func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func CheckPassword(hash, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return domain.ErrUnauthorized
	}
	return nil
}

func ValidatePassword(password string) error {
	if len(password) < minPasswordLen {
		return domain.Invalid("пароль должен быть не короче 8 символов")
	}
	if len(password) > maxPasswordLen {
		return domain.Invalid("пароль должен быть не длиннее 72 символов")
	}
	return nil
}
