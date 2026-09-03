package mailer

import (
	"context"
	"log/slog"

	"sade/config"
)

// logMailer renders each message to the logger instead of sending it. The
// plain-text body is logged at Info (so a magic link is visible during
// local development); the full RFC 5322 message at Debug.
type logMailer struct {
	cfg config.MailerConfig
	log *slog.Logger
}

func (m *logMailer) Send(_ context.Context, msg Message) error {
	raw, err := build(m.cfg, msg)
	if err != nil {
		return err
	}
	m.log.Info("email (log transport - not sent)",
		"from", m.cfg.FromAddr, "to", msg.To, "subject", msg.Subject, "body", msg.Text)
	m.log.Debug("email raw message", "rfc5322", string(raw))
	return nil
}
