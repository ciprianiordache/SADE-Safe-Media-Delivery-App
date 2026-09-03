package magic_token

import "errors"

var (
	// ErrNotFound is returned when no unconsumed token matches the hash.
	ErrNotFound = errors.New("magic_token: not found")
	// ErrExpired is returned when a matching token exists but is past its
	// expiry.
	ErrExpired = errors.New("magic_token: expired")
)
