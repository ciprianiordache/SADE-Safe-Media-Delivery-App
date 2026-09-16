package mailer

import "context"

// Message is one outbound email. SADE always sends to a single recipient
// (a magic link to an operator, a preview link to a client).
type Message struct {
	To      string // recipient address
	Subject string
	Text    string // plain-text body (required)
	HTML    string // optional; when set the message is multipart/alternative
	Inline  []Inline
}

// Inline is an image (or other asset) embedded in the message itself and
// referenced from HTML via "cid:<CID>", rather than a remote URL the
// recipient's mail client would have to fetch. Gmail and most webmail
// providers proxy remote images through their own servers, which can't
// reach a private/LAN address (relevant while SADE runs without a public
// domain); an inline part sidesteps that entirely and is the standard way
// transactional email ships a logo. Ignored when Message.HTML is empty.
type Inline struct {
	CID         string // referenced in HTML as cid:<CID>
	ContentType string // e.g. "image/png"
	Data        []byte
}

// Mailer sends a Message. Implementations are chosen by
// MailerConfig.Transport: smtpMailer, logMailer, noopMailer.
type Mailer interface {
	Send(ctx context.Context, msg Message) error
}
