package mailer

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/quotedprintable"
	"net"
	"strings"
	"testing"
	"time"

	"sade/config"
)

func baseCfg() config.MailerConfig {
	return config.MailerConfig{
		Transport: "log",
		FromName:  "Safe Media Delivery",
		FromAddr:  "no-reply@sade.local",
	}
}

func TestNewSelectsTransport(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	for _, tc := range []struct {
		transport string
		want      string
	}{
		{"smtp", "*mailer.smtpMailer"},
		{"log", "*mailer.logMailer"},
		{"noop", "mailer.noopMailer"},
	} {
		cfg := baseCfg()
		cfg.Transport = tc.transport
		m, err := New(cfg, log)
		if err != nil {
			t.Fatalf("New(%q): %v", tc.transport, err)
		}
		if got := fmt.Sprintf("%T", m); got != tc.want {
			t.Errorf("New(%q) = %s, want %s", tc.transport, got, tc.want)
		}
	}

	if _, err := New(config.MailerConfig{Transport: "carrier-pigeon"}, log); err == nil {
		t.Fatal("expected error for unknown transport")
	}
}

func TestBuildPlainText(t *testing.T) {
	raw, err := build(baseCfg(), Message{
		To: "op@example.com", Subject: "Sign in to SADE", Text: "Open https://sade.local/x to sign in.",
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{
		"From: \"Safe Media Delivery\" <no-reply@sade.local>\r\n",
		"To: <op@example.com>\r\n",
		"Subject: Sign in to SADE\r\n",
		"MIME-Version: 1.0\r\n",
		"Content-Type: text/plain; charset=utf-8\r\n",
		"Content-Transfer-Encoding: quoted-printable\r\n",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing header %q in:\n%s", want, s)
		}
	}
	headers, body, _ := strings.Cut(s, "\r\n\r\n")
	if strings.Contains(headers, "boundary=") {
		t.Errorf("plain-text message should not be multipart:\n%s", headers)
	}
	dec, _ := io.ReadAll(quotedprintable.NewReader(strings.NewReader(body)))
	if !strings.Contains(string(dec), "https://sade.local/x") {
		t.Errorf("body did not round-trip through quoted-printable: %q", dec)
	}
}

func TestBuildMultipartWhenHTML(t *testing.T) {
	raw, err := build(baseCfg(), Message{
		To: "c@example.com", Subject: "Préview ready", // non-ASCII -> encoded-word
		Text: "plain", HTML: "<p>hi</p>",
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, "Content-Type: multipart/alternative; boundary=") {
		t.Errorf("expected multipart/alternative:\n%s", s)
	}
	if !strings.Contains(s, "=?utf-8?q?") && !strings.Contains(s, "=?UTF-8?q?") {
		t.Errorf("non-ASCII subject should be RFC 2047 encoded: %s", s)
	}
	if strings.Count(s, "Content-Type: text/plain") != 1 || strings.Count(s, "Content-Type: text/html") != 1 {
		t.Errorf("expected one text and one html part:\n%s", s)
	}
}

func TestBuildRejectsIncompleteMessages(t *testing.T) {
	for _, tc := range []struct {
		name string
		msg  Message
		want error
	}{
		{"no recipient", Message{Subject: "s", Text: "t"}, ErrNoRecipient},
		{"no subject", Message{To: "a@b.c", Text: "t"}, ErrNoSubject},
		{"no body", Message{To: "a@b.c", Subject: "s"}, ErrEmptyBody},
	} {
		if _, err := build(baseCfg(), tc.msg); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestLogTransportEmitsBodyAtInfo(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	m, _ := New(baseCfg(), log)

	err := m.Send(context.Background(), Message{
		To: "op@example.com", Subject: "hello", Text: "magic: https://sade.local/cb?token=abc",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "op@example.com") || !strings.Contains(out, "https://sade.local/cb?token=abc") {
		t.Errorf("log transport did not surface recipient + body:\n%s", out)
	}
}

func TestNoopTransportSendsNothing(t *testing.T) {
	m, _ := New(config.MailerConfig{Transport: "noop"}, nil)
	if err := m.Send(context.Background(), Message{To: "x@y.z", Subject: "s", Text: "t"}); err != nil {
		t.Fatalf("noop Send: %v", err)
	}
}

func TestSMTPTransportDelivers(t *testing.T) {
	addr, received := startFakeSMTP(t)
	host, portStr, _ := net.SplitHostPort(addr)

	cfg := baseCfg()
	cfg.Transport = "smtp"
	cfg.SMTP = config.SMTPConfig{Host: host, Port: atoi(portStr)}
	m, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	msg := Message{To: "client@example.com", Subject: "Preview ready", Text: "link inside"}
	if err := m.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case got := <-received:
		if !strings.Contains(got, "To: <client@example.com>") || !strings.Contains(got, "Subject: Preview ready") {
			t.Errorf("server did not receive expected headers:\n%s", got)
		}
		dec, _ := io.ReadAll(quotedprintable.NewReader(strings.NewReader(got)))
		if !strings.Contains(string(dec), "link inside") {
			t.Errorf("body not delivered: %q", dec)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("fake SMTP server received nothing")
	}
}

// startFakeSMTP spins a one-shot plaintext SMTP server (no STARTTLS, no auth)
// and returns its address plus a channel that yields the DATA payload.
func startFakeSMTP(t *testing.T) (string, <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	out := make(chan string, 1)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		reply := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }

		reply("220 fake ESMTP")
		var body strings.Builder
		inData := false
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				if line == ".\r\n" {
					inData = false
					reply("250 2.0.0 Ok: queued")
					out <- body.String()
					continue
				}
				body.WriteString(line)
				continue
			}
			switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				reply("250-fake greets you")
				reply("250 SIZE 10485760")
			case cmd == "DATA":
				reply("354 End data with <CR><LF>.<CR><LF>")
				inData = true
			case cmd == "QUIT":
				reply("221 2.0.0 Bye")
				return
			default: // MAIL FROM, RCPT TO, RSET, ...
				reply("250 2.1.0 Ok")
			}
		}
	}()

	return ln.Addr().String(), out
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}
