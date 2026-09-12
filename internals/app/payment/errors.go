package payment

import "errors"

var (
	// ErrNotFound is returned when no payment matches the lookup, or a
	// token resolves to an asset/job that has none.
	ErrNotFound = errors.New("payment: not found")
	// ErrDisabled is returned by Service.Checkout when config.PaymentConfig
	// has no Stripe secret key - the feature is off, not broken.
	ErrDisabled = errors.New("payment: disabled")
	// ErrInvalidToken is returned when the caller's preview token doesn't
	// verify (unknown, expired, wrong purpose, or points at a non-preview
	// asset).
	ErrInvalidToken = errors.New("payment: invalid token")
)
