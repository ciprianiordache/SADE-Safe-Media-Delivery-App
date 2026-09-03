package mailer

import "context"

// Message is one outbound email. SADE always sends to a single recipient
// (a magic link to an operator, a preview link to a client).
type Message struct {
	To      string // recipient address
	Subject string
	Text    string // plain-text body (required)
	HTML    string // optional; when set the message is multipart/alternative
}

// Mailer sends a Message. Implementations are chosen by
// MailerConfig.Transport: smtpMailer, logMailer, noopMailer.
type Mailer interface {
	Send(ctx context.Context, msg Message) error
}
