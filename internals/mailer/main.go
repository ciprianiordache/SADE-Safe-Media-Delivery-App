// Package mailer sends outbound email. The transport is chosen by
// MailerConfig.Transport:
//
//	smtp - deliver over SMTP (STARTTLS on 587, implicit TLS on 465)
//	log  - render each message to the logger without sending it; this is
//	       how magic-link login works locally with no SMTP account
//	noop - discard silently
//
// The RFC 5322 message (headers, quoted-printable body, multipart/alternative
// when HTML is present) is assembled once in build, so every transport would
// put identical bytes on the wire.
package mailer

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"time"

	"sade/config"
)

// New builds the Mailer named by cfg.Transport. config.Validate has already
// checked that an "smtp" transport has host/port/user/password set.
func New(cfg config.MailerConfig, log *slog.Logger) (Mailer, error) {
	switch cfg.Transport {
	case "smtp":
		return &smtpMailer{cfg: cfg, log: log}, nil
	case "log":
		return &logMailer{cfg: cfg, log: log}, nil
	case "noop":
		return noopMailer{}, nil
	default:
		return nil, fmt.Errorf("mailer: unknown transport %q (want smtp, log or noop)", cfg.Transport)
	}
}

// build assembles the full RFC 5322 message bytes (CRLF line endings).
func build(cfg config.MailerConfig, msg Message) ([]byte, error) {
	switch {
	case strings.TrimSpace(msg.To) == "":
		return nil, ErrNoRecipient
	case strings.TrimSpace(msg.Subject) == "":
		return nil, ErrNoSubject
	case msg.Text == "" && msg.HTML == "":
		return nil, ErrEmptyBody
	}

	var b bytes.Buffer
	header := func(k, v string) { fmt.Fprintf(&b, "%s: %s\r\n", k, v) }
	header("From", (&mail.Address{Name: cfg.FromName, Address: cfg.FromAddr}).String())
	header("To", (&mail.Address{Address: msg.To}).String())
	header("Subject", mime.QEncoding.Encode("utf-8", msg.Subject))
	header("Date", time.Now().Format(time.RFC1123Z))
	header("Message-ID", messageID(cfg.FromAddr))
	header("MIME-Version", "1.0")

	if msg.HTML == "" {
		header("Content-Type", "text/plain; charset=utf-8")
		header("Content-Transfer-Encoding", "quoted-printable")
		b.WriteString("\r\n")
		writeQP(&b, msg.Text)
		return b.Bytes(), nil
	}

	text := msg.Text
	if text == "" {
		text = "This message needs an HTML-capable email client."
	}
	altBoundary := randToken(24)

	if len(msg.Inline) == 0 {
		header("Content-Type", "multipart/alternative; boundary="+altBoundary)
		b.WriteString("\r\n")
		writePart(&b, altBoundary, "text/plain; charset=utf-8", text)
		writePart(&b, altBoundary, "text/html; charset=utf-8", msg.HTML)
		fmt.Fprintf(&b, "--%s--\r\n", altBoundary)
		return b.Bytes(), nil
	}

	// HTML references its images as cid:<CID>, so the alternative part (the
	// text/html and text/plain choice) nests inside an outer multipart/related
	// (the HTML plus the assets it points to).
	relBoundary := randToken(24)
	header("Content-Type", fmt.Sprintf(`multipart/related; type="multipart/alternative"; boundary=%s`, relBoundary))
	b.WriteString("\r\n")

	fmt.Fprintf(&b, "--%s\r\n", relBoundary)
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%s\r\n\r\n", altBoundary)
	writePart(&b, altBoundary, "text/plain; charset=utf-8", text)
	writePart(&b, altBoundary, "text/html; charset=utf-8", msg.HTML)
	fmt.Fprintf(&b, "--%s--\r\n\r\n", altBoundary)

	for _, img := range msg.Inline {
		fmt.Fprintf(&b, "--%s\r\n", relBoundary)
		fmt.Fprintf(&b, "Content-Type: %s\r\n", img.ContentType)
		b.WriteString("Content-Transfer-Encoding: base64\r\n")
		fmt.Fprintf(&b, "Content-ID: <%s>\r\n", img.CID)
		fmt.Fprintf(&b, "Content-Disposition: inline; filename=%q\r\n\r\n", img.CID)
		writeBase64Lines(&b, img.Data)
		b.WriteString("\r\n")
	}
	fmt.Fprintf(&b, "--%s--\r\n", relBoundary)
	return b.Bytes(), nil
}

func writePart(b *bytes.Buffer, boundary, contentType, body string) {
	fmt.Fprintf(b, "--%s\r\n", boundary)
	fmt.Fprintf(b, "Content-Type: %s\r\n", contentType)
	b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	writeQP(b, body)
	b.WriteString("\r\n")
}

func writeQP(b *bytes.Buffer, s string) {
	w := quotedprintable.NewWriter(b)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
}

// writeBase64Lines base64-encodes data and wraps it at RFC 2045's 76-column
// limit for encoded body content - a stricter mail server can reject or
// mangle longer lines.
func writeBase64Lines(b *bytes.Buffer, data []byte) {
	const lineLen = 76
	enc := base64.StdEncoding.EncodeToString(data)
	for i := 0; i < len(enc); i += lineLen {
		end := min(i+lineLen, len(enc))
		b.WriteString(enc[i:end])
		b.WriteString("\r\n")
	}
}

func messageID(fromAddr string) string {
	domain := "localhost"
	if i := strings.LastIndex(fromAddr, "@"); i >= 0 && i+1 < len(fromAddr) {
		domain = fromAddr[i+1:]
	}
	return fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), randToken(12), domain)
}

func randToken(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
