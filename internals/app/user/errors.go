package user

import "errors"

var (
	// ErrNotFound is returned when no account matches the lookup.
	ErrNotFound = errors.New("user: not found")
	// ErrInvalidInput is returned for a malformed email or an unknown role.
	ErrInvalidInput = errors.New("user: invalid input")
)
