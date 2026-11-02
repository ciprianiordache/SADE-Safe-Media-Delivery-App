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
	// ErrOtherBrowser is returned by Complete for a valid link opened in a
	// browser other than the one that requested it. The link stays usable
	// from the right browser, and the code from the same email works there.
	ErrOtherBrowser = errors.New("auth: link requested from another browser")
	// ErrInvalidCode is returned by CompleteWithCode for a wrong, expired or
	// foreign code - one error for all, so a caller cannot probe which.
	ErrInvalidCode = errors.New("auth: invalid or expired code")
	// ErrPasskeysDisabled: passkeys are not configured on this server.
	ErrPasskeysDisabled = errors.New("auth: passkeys are not available")
	// ErrInvalidPasskey: a passkey ceremony failed (unknown or expired
	// ceremony, bad signature, unknown credential).
	ErrInvalidPasskey = errors.New("auth: passkey verification failed")
	// ErrNoPasskey: "require a passkey" cannot be turned on for an account
	// with no passkey - it would lock the owner out.
	ErrNoPasskey = errors.New("auth: the account has no passkey")
	// ErrNotFound: an unknown session or passkey, or one owned by another
	// account.
	ErrNotFound = errors.New("auth: not found")
)
