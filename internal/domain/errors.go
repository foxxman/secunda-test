package domain

import "errors"

var (
	ErrInvalid      = errors.New("invalid")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
)

type InvalidError struct {
	Msg string
}

func (e InvalidError) Error() string {
	return e.Msg
}

func (e InvalidError) Unwrap() error {
	return ErrInvalid
}

func Invalid(msg string) error {
	return InvalidError{Msg: msg}
}
