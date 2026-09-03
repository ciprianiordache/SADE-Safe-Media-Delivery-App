package mailer

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strconv"
	"time"

	"sade/config"
)

// smtpExchangeTimeout bounds the whole SMTP conversation when the caller's
// context carries no deadline.
const smtpExchangeTimeout = 30 * time.Second

// smtpMailer delivers over SMTP. Port 465 uses implicit TLS; any other port
// uses STARTTLS when the server advertises it. Auth (PLAIN) is attempted only
// when a username is configured.
type smtpMailer struct {
	cfg config.MailerConfig
	log *slog.Logger
}

func (m *smtpMailer) Send(ctx context.Context, msg Message) error {
	raw, err := build(m.cfg, msg)
	if err != nil {
		return err
	}

	s := m.cfg.SMTP
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))

	conn, err := (&net.Dialer{Timeout: smtpExchangeTimeout}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("mailer: dial %s: %w", addr, err)
	}
	deadline := time.Now().Add(smtpExchangeTimeout)
	if d, ok := ctx.Deadline(); ok {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)

	if s.Port == 465 {
		conn = tls.Client(conn, &tls.Config{ServerName: s.Host})
	}

	client, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("mailer: smtp handshake with %s: %w", addr, err)
	}
	defer client.Close()

	if s.Port != 465 {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: s.Host}); err != nil {
				return fmt.Errorf("mailer: starttls: %w", err)
			}
		}
	}
	if s.User != "" {
		if err := client.Auth(smtp.PlainAuth("", s.User, s.Password, s.Host)); err != nil {
			return fmt.Errorf("mailer: auth as %s: %w", s.User, err)
		}
	}

	if err := client.Mail(m.cfg.FromAddr); err != nil {
		return fmt.Errorf("mailer: MAIL FROM <%s>: %w", m.cfg.FromAddr, err)
	}
	if err := client.Rcpt(msg.To); err != nil {
		return fmt.Errorf("mailer: RCPT TO <%s>: %w", msg.To, err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("mailer: DATA: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("mailer: write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mailer: finish body: %w", err)
	}

	m.log.Info("email sent", "to", msg.To, "subject", msg.Subject)
	return client.Quit()
}
