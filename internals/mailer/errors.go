package mailer

import "errors"

var (
	ErrNoRecipient = errors.New("mailer: message has no recipient")
	ErrNoSubject   = errors.New("mailer: message has no subject")
	ErrEmptyBody   = errors.New("mailer: message has no body")
)
