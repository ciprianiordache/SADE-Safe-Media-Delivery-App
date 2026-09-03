package session

import "errors"

// ErrNotFound is returned when no session matches the hash.
var ErrNotFound = errors.New("session: not found")
