package asset

import "errors"

var (
	// ErrNotFound is returned when no asset matches the lookup.
	ErrNotFound = errors.New("asset: not found")
	// ErrInvalidInput is returned for a malformed asset (unknown kind,
	// empty storage key).
	ErrInvalidInput = errors.New("asset: invalid input")
)
