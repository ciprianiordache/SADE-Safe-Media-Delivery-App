package magic_token

import "errors"

var (
	// ErrNotFound is returned when no unconsumed token matches the hash.
	ErrNotFound = errors.New("magic_token: not found")
	// ErrExpired is returned when a matching token exists but is past its
	// expiry.
	ErrExpired = errors.New("magic_token: expired")
	// ErrOtherBrowser is returned by Consume when the token is valid but the
	// request did not come from the browser that asked for it. The token is
	// left in place - the right browser can still use it.
	ErrOtherBrowser = errors.New("magic_token: requested from another browser")
	// ErrBadCode is returned by ConsumeByCode for a wrong code.
	ErrBadCode = errors.New("magic_token: wrong code")
)
