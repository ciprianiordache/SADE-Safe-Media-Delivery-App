package storage

import "errors"

var (
	// ErrNotFound is returned by Open and Stat when the key does not exist.
	ErrNotFound = errors.New("storage: object not found")
	// ErrBadKey is returned when a key is empty, absolute, or escapes the
	// backend root (contains "..").
	ErrBadKey = errors.New("storage: invalid key")
)
