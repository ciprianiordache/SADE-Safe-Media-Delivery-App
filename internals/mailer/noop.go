package mailer

import "context"

// noopMailer discards every message. Useful in tests and for environments
// that must not send mail at all.
type noopMailer struct{}

func (noopMailer) Send(context.Context, Message) error { return nil }
