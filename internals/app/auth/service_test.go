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
	"sade/internals/app/login_event"
	"sade/internals/app/magic_token"
	"sade/internals/app/passkey"
	"sade/internals/app/session"
	"sade/internals/app/user"
	"sade/internals/mailer"
	"sade/internals/ratelimit"
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

func newService(t *testing.T, opts ...func(*config.AuthConfig)) (*Service, *captureMailer) {
	t.Helper()
	db := testutil.DB(t, user.User{}, magic_token.MagicToken{}, session.Session{}, login_event.LoginEvent{}, passkey.Passkey{})
	cm := &captureMailer{}
	cfg := config.AuthConfig{
		MagicLinkTTL:      config.Duration(15 * time.Minute),
		SessionTTL:        config.Duration(24 * time.Hour),
		SessionCookieName: "sade_session",
	}
	for _, o := range opts {
		o(&cfg)
	}
	svc := NewService(
		user.NewService(user.NewRepo(db), testutil.Logger()),
		magic_token.NewRepo(db), session.NewRepo(db), cm,
		cfg, "http://api.test", "http://app.test", testutil.Logger(),
	)
	svc.UseLoginEvents(login_event.NewRepo(db))
	svc.UsePasskeys(passkey.NewRepo(db), nil) // repo ready; tests that need passkeys add a verifier
	svc.async = func(f func()) { f() }        // alert emails land before the call returns
	return svc, cm
}

// testNonce stands in for the sign-in cookie of "this browser".
const testNonce = "test-browser-nonce-0001"

// completeT signs in with a link from this browser and returns the session.
func completeT(svc *Service, ctx context.Context, tok string) (string, time.Time, error) {
	res, err := svc.Complete(ctx, tok, testNonce, Client{UserAgent: "Mozilla/5.0 (Windows NT 10.0) Chrome/130.0", IP: "203.0.113.5"})
	return res.SessionSecret, res.ExpiresAt, err
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

	if err := svc.RequestLink(context.Background(), "  Op@Example.com ", testNonce); err != nil {
		t.Fatalf("RequestLink: %v", err)
	}
	msg := cm.last()
	if msg.To != "op@example.com" {
		t.Errorf("mail To = %q, want op@example.com", msg.To)
	}
	if !strings.Contains(msg.Text, "http://app.test/auth/confirm#token=") {
		t.Errorf("confirmation link missing from body:\n%s", msg.Text)
	}

	secret, exp, err := completeT(svc, context.Background(), linkToken(t, cm))
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
	if err := svc.RequestLink(context.Background(), "not-an-email", testNonce); !errors.Is(err, ErrInvalidEmail) {
		t.Errorf("RequestLink(bad) = %v, want ErrInvalidEmail", err)
	}
}

func TestCompleteRejectsBadTokens(t *testing.T) {
	svc, cm := newService(t)
	_ = svc.RequestLink(context.Background(), "a@b.c", testNonce)
	good := linkToken(t, cm)

	if _, _, err := completeT(svc, context.Background(), "garbage"); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Complete(garbage) = %v, want ErrInvalidToken", err)
	}
	// first use ok
	if _, _, err := completeT(svc, context.Background(), good); err != nil {
		t.Fatalf("Complete(good): %v", err)
	}
	// reuse rejected
	if _, _, err := completeT(svc, context.Background(), good); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Complete(reused) = %v, want ErrInvalidToken", err)
	}
}

func TestCompleteRejectsExpiredToken(t *testing.T) {
	svc, cm := newService(t)
	svc.cfg.MagicLinkTTL = config.Duration(-time.Second) // already expired when minted
	_ = svc.RequestLink(context.Background(), "a@b.c", testNonce)

	if _, _, err := completeT(svc, context.Background(), linkToken(t, cm)); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Complete(expired) = %v, want ErrInvalidToken", err)
	}
}

func TestAuthenticateAndLogout(t *testing.T) {
	svc, cm := newService(t)
	_ = svc.RequestLink(context.Background(), "a@b.c", testNonce)
	secret, _, _ := completeT(svc, context.Background(), linkToken(t, cm))

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
	_ = svc.RequestLink(context.Background(), "a@b.c", testNonce)
	secret, _, err := completeT(svc, context.Background(), linkToken(t, cm))
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

func TestPeekDoesNotSpendTheToken(t *testing.T) {
	svc, cm := newService(t)
	ctx := context.Background()
	if err := svc.RequestLink(ctx, "op@example.com", testNonce); err != nil {
		t.Fatal(err)
	}
	tok := linkToken(t, cm)

	for i := 0; i < 2; i++ {
		email, same, err := svc.Peek(ctx, tok, testNonce)
		if err != nil || email != "op@example.com" || !same {
			t.Fatalf("Peek #%d = %q, %v, %v", i, email, same, err)
		}
	}
	if _, _, err := completeT(svc, ctx, tok); err != nil {
		t.Fatalf("Complete after Peek: %v", err)
	}
	if _, _, err := svc.Peek(ctx, tok, testNonce); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Peek of a spent token = %v, want ErrInvalidToken", err)
	}
}

func TestSignupPolicy(t *testing.T) {
	ctx := context.Background()

	closed, cm := newService(t, func(c *config.AuthConfig) { c.SignupMode = "closed" })
	if err := closed.RequestLink(ctx, "new@example.com", testNonce); err != nil {
		t.Fatalf("closed signup must look like success, got %v", err)
	}
	if len(cm.sent) != 0 {
		t.Errorf("closed signup sent %d emails", len(cm.sent))
	}
	if _, err := closed.users.FindByEmail("new@example.com"); !errors.Is(err, user.ErrNotFound) {
		t.Errorf("closed signup created an account: %v", err)
	}

	allow, cm := newService(t, func(c *config.AuthConfig) {
		c.SignupMode = "allowlist"
		c.SignupAllowedDomains = []string{"@studio.test"}
		c.SignupAllowedEmails = []string{"Boss@Elsewhere.test"}
	})
	for _, ok := range []string{"a@studio.test", "boss@elsewhere.test"} {
		if err := allow.RequestLink(ctx, ok, testNonce); err != nil {
			t.Fatalf("RequestLink(%s): %v", ok, err)
		}
	}
	if err := allow.RequestLink(ctx, "x@notstudio.test", testNonce); err != nil {
		t.Fatalf("refused address must look like success, got %v", err)
	}
	if len(cm.sent) != 2 {
		t.Errorf("allowlist sent %d emails, want 2", len(cm.sent))
	}

	// An existing account always signs in, whatever the mode.
	if _, err := closed.users.EnsureByEmail("old@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := closed.RequestLink(ctx, "old@example.com", testNonce); err != nil {
		t.Fatal(err)
	}
}

func TestPerAddressLimit(t *testing.T) {
	svc, cm := newService(t)
	svc.SetEmailLimiter(ratelimit.New(2, time.Hour))
	for i := 0; i < 4; i++ {
		if err := svc.RequestLink(context.Background(), "op@example.com", testNonce); err != nil {
			t.Fatalf("RequestLink #%d: %v", i, err)
		}
	}
	if len(cm.sent) != 2 {
		t.Errorf("sent %d emails to one address, want 2", len(cm.sent))
	}
}

func TestRevokeAllEndsEverySession(t *testing.T) {
	svc, cm := newService(t)
	ctx := context.Background()
	var secrets []string
	for i := 0; i < 2; i++ {
		if err := svc.RequestLink(ctx, "op@example.com", testNonce); err != nil {
			t.Fatal(err)
		}
		sec, _, err := completeT(svc, ctx, linkToken(t, cm))
		if err != nil {
			t.Fatal(err)
		}
		secrets = append(secrets, sec)
	}
	u, err := svc.Authenticate(ctx, secrets[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RevokeAll(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	for _, sec := range secrets {
		if _, err := svc.Authenticate(ctx, sec); err == nil {
			t.Errorf("session survived RevokeAll")
		}
	}
}

func TestPurgeExpiredRemovesOnlyExpiredRows(t *testing.T) {
	svc, cm := newService(t)
	ctx := context.Background()

	// One live session, one expired session, one expired unused link.
	if err := svc.RequestLink(ctx, "live@example.com", testNonce); err != nil {
		t.Fatal(err)
	}
	live, _, err := completeT(svc, ctx, linkToken(t, cm))
	if err != nil {
		t.Fatal(err)
	}
	svc.cfg.SessionTTL = config.Duration(-time.Minute)
	svc.cfg.MagicLinkTTL = config.Duration(-time.Minute)
	if err := svc.RequestLink(ctx, "old@example.com", testNonce); err != nil {
		t.Fatal(err)
	}
	expiredTok := linkToken(t, cm)
	if err := svc.RequestLink(ctx, "old2@example.com", testNonce); err != nil {
		t.Fatal(err)
	}
	_ = expiredTok

	if err := svc.PurgeExpired(ctx); err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}
	if _, err := svc.Authenticate(ctx, live); err != nil {
		t.Errorf("live session was purged: %v", err)
	}
	if n, err := svc.tokens.DeleteExpired(time.Now()); err != nil || n != 0 {
		t.Errorf("expired tokens left after purge: %d (%v)", n, err)
	}
}
