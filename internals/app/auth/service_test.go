package auth

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"sade/config"
	"sade/internals/app/magic_token"
	"sade/internals/app/session"
	"sade/internals/app/user"
	"sade/internals/mailer"
	"sade/internals/testutil"
)

// captureMailer records every message instead of sending it.
type captureMailer struct {
	mu   sync.Mutex
	sent []mailer.Message
}

func (m *captureMailer) Send(_ context.Context, msg mailer.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}

func (m *captureMailer) last() mailer.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sent[len(m.sent)-1]
}

func newService(t *testing.T) (*Service, *captureMailer) {
	t.Helper()
	db := testutil.DB(t, user.User{}, magic_token.MagicToken{}, session.Session{})
	cm := &captureMailer{}
	cfg := config.AuthConfig{
		MagicLinkTTL:      config.Duration(15 * time.Minute),
		SessionTTL:        config.Duration(24 * time.Hour),
		SessionCookieName: "sade_session",
	}
	svc := NewService(
		user.NewService(user.NewRepo(db), testutil.Logger()),
		magic_token.NewRepo(db), session.NewRepo(db), cm,
		cfg, "http://api.test", "http://app.test", testutil.Logger(),
	)
	return svc, cm
}

// linkToken pulls the ?token= value out of the most recent email.
func linkToken(t *testing.T, cm *captureMailer) string {
	t.Helper()
	body := cm.last().Text
	i := strings.Index(body, "token=")
	if i < 0 {
		t.Fatalf("no token in email body:\n%s", body)
	}
	raw := body[i+len("token="):]
	if j := strings.IndexAny(raw, " \n\r"); j >= 0 {
		raw = raw[:j]
	}
	dec, err := url.QueryUnescape(raw)
	if err != nil {
		t.Fatalf("token not url-decodable: %v", err)
	}
	return dec
}

func TestRequestLinkThenComplete(t *testing.T) {
	svc, cm := newService(t)

	if err := svc.RequestLink(context.Background(), "  Op@Example.com "); err != nil {
		t.Fatalf("RequestLink: %v", err)
	}
	msg := cm.last()
	if msg.To != "op@example.com" {
		t.Errorf("mail To = %q, want op@example.com", msg.To)
	}
	if !strings.Contains(msg.Text, "http://api.test/api/auth/callback?token=") {
		t.Errorf("callback link missing from body:\n%s", msg.Text)
	}

	secret, exp, err := svc.Complete(context.Background(), linkToken(t, cm))
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if secret == "" || exp.Before(time.Now()) {
		t.Fatalf("bad session: secret=%q exp=%s", secret, exp)
	}

	u, err := svc.Authenticate(context.Background(), secret)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if u.Email != "op@example.com" || u.Role != user.RoleOperator {
		t.Errorf("session user = %+v", u)
	}
}

func TestRequestLinkRejectsBadEmail(t *testing.T) {
	svc, _ := newService(t)
	if err := svc.RequestLink(context.Background(), "not-an-email"); !errors.Is(err, ErrInvalidEmail) {
		t.Errorf("RequestLink(bad) = %v, want ErrInvalidEmail", err)
	}
}

func TestCompleteRejectsBadTokens(t *testing.T) {
	svc, cm := newService(t)
	_ = svc.RequestLink(context.Background(), "a@b.c")
	good := linkToken(t, cm)

	if _, _, err := svc.Complete(context.Background(), "garbage"); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Complete(garbage) = %v, want ErrInvalidToken", err)
	}
	// first use ok
	if _, _, err := svc.Complete(context.Background(), good); err != nil {
		t.Fatalf("Complete(good): %v", err)
	}
	// reuse rejected
	if _, _, err := svc.Complete(context.Background(), good); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Complete(reused) = %v, want ErrInvalidToken", err)
	}
}

func TestCompleteRejectsExpiredToken(t *testing.T) {
	svc, cm := newService(t)
	svc.cfg.MagicLinkTTL = config.Duration(-time.Second) // already expired when minted
	_ = svc.RequestLink(context.Background(), "a@b.c")

	if _, _, err := svc.Complete(context.Background(), linkToken(t, cm)); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Complete(expired) = %v, want ErrInvalidToken", err)
	}
}

func TestAuthenticateAndLogout(t *testing.T) {
	svc, cm := newService(t)
	_ = svc.RequestLink(context.Background(), "a@b.c")
	secret, _, _ := svc.Complete(context.Background(), linkToken(t, cm))

	if _, err := svc.Authenticate(context.Background(), ""); !errors.Is(err, ErrNoSession) {
		t.Errorf("Authenticate(empty) = %v, want ErrNoSession", err)
	}
	if _, err := svc.Authenticate(context.Background(), "unknown"); !errors.Is(err, ErrNoSession) {
		t.Errorf("Authenticate(unknown) = %v, want ErrNoSession", err)
	}

	if err := svc.Logout(context.Background(), secret); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := svc.Authenticate(context.Background(), secret); !errors.Is(err, ErrNoSession) {
		t.Errorf("Authenticate after logout = %v, want ErrNoSession", err)
	}
	// idempotent
	if err := svc.Logout(context.Background(), secret); err != nil {
		t.Errorf("Logout (2nd) = %v, want nil", err)
	}
}

func TestAuthenticateExpiredSessionIsCleared(t *testing.T) {
	svc, cm := newService(t)
	svc.cfg.SessionTTL = config.Duration(-time.Second)
	_ = svc.RequestLink(context.Background(), "a@b.c")
	secret, _, err := svc.Complete(context.Background(), linkToken(t, cm))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Authenticate(context.Background(), secret); !errors.Is(err, ErrSessionExpired) {
		t.Errorf("Authenticate(expired) = %v, want ErrSessionExpired", err)
	}
	// row removed as a side effect -> now looks like no session
	if _, err := svc.Authenticate(context.Background(), secret); !errors.Is(err, ErrNoSession) {
		t.Errorf("Authenticate(after cleanup) = %v, want ErrNoSession", err)
	}
}
