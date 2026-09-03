package auth

import "errors"

var (
	// ErrInvalidEmail is returned by RequestLink for a malformed address.
	ErrInvalidEmail = errors.New("auth: invalid email address")
	// ErrInvalidToken is returned by Complete when the magic-link token is
	// unknown, already used, or expired - the same error for all three so a
	// caller cannot probe which.
	ErrInvalidToken = errors.New("auth: invalid or expired link")
	// ErrNoSession is returned by Authenticate when the cookie is missing,
	// unknown, or its user no longer exists.
	ErrNoSession = errors.New("auth: no session")
	// ErrSessionExpired is returned by Authenticate when the session exists
	// but has lapsed (the row is deleted as a side effect).
	ErrSessionExpired = errors.New("auth: session expired")
)
