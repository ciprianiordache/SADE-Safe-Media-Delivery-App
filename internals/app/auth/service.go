// Package auth is the sign-in flow. It has no table of its own: it composes
// the user, magic_token, session, login_event and passkey domains plus the
// mailer.
//
//	RequestLink      -> ensure an account, mint a link + 6-digit code bound
//	                    to the requesting browser, email them
//	Complete         -> spend the link (same browser only), start a session
//	CompleteWithCode -> spend the code (same browser only), start a session
//	Begin/FinishPasskey* -> passkey sign-in, alone or as the second step
//	Authenticate     -> resolve a session secret to its user
//	Logout           -> drop the session
//
// Only hashes of the link token, the code and the session secret are stored,
// so a database leak yields neither working links nor live sessions.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"sade/config"
	"sade/internals/app/login_event"
	"sade/internals/app/magic_token"
	"sade/internals/app/passkey"
	"sade/internals/app/session"
	"sade/internals/app/user"
	"sade/internals/emailtmpl"
	"sade/internals/mailer"
	"sade/internals/ratelimit"
	"sade/internals/token"

	"github.com/go-webauthn/webauthn/webauthn"
)

// maxCodeAttempts is how many wrong codes a sign-in email survives.
const maxCodeAttempts = 5

// touchEvery bounds how often a session's LastSeenAt is written - once per
// window, not on every request.
const touchEvery = 5 * time.Minute

// Client is who is signing in, for the session record and the alert email.
type Client struct {
	UserAgent string
	IP        string
}

// SignIn is the outcome of a successful email step (link or code). Either a
// session was started (SessionSecret/ExpiresAt), or the account requires a
// passkey too and Passkey carries the challenge for that second step.
type SignIn struct {
	SessionSecret string
	ExpiresAt     time.Time
	Passkey       *PasskeyChallenge
}

// Service runs the sign-in flow.
type Service struct {
	users       user.Service
	tokens      magic_token.Repo
	sessions    session.Repo
	mail        mailer.Mailer
	cfg         config.AuthConfig
	publicURL   string // API base (config.App.PublicURL)
	frontendURL string
	log         *slog.Logger

	// emailLimiter caps sign-in links per recipient address, on top of the
	// per-IP limit in the router: an attacker rotating IPs still cannot
	// flood one mailbox. Nil disables it (see SetEmailLimiter).
	emailLimiter *ratelimit.Limiter

	// Optional parts, each nil-safe so tests can leave them out.
	events     login_event.Repo // sign-in history + new-device detection
	passkeys   passkey.Repo
	webauthn   *webauthn.WebAuthn // nil: passkeys disabled
	ceremonies *ceremonyStore
	signer     *token.Signer // signs the alert email's "this wasn't me" link

	// async runs fire-and-forget work (the alert email) so it never slows a
	// sign-in down. Tests swap in a synchronous version.
	async func(func())
}

// SetEmailLimiter installs the per-address cap on RequestLink.
func (s *Service) SetEmailLimiter(l *ratelimit.Limiter) { s.emailLimiter = l }

// UseLoginEvents turns on sign-in history and new-device alerts.
func (s *Service) UseLoginEvents(r login_event.Repo) { s.events = r }

// UseAlertSigner signs the "this wasn't me" link in new-device alerts.
func (s *Service) UseAlertSigner(sg *token.Signer) { s.signer = sg }

// UsePasskeys turns on passkey registration and sign-in. wa nil leaves them
// off (e.g. the frontend runs on a bare IP, which WebAuthn does not allow).
func (s *Service) UsePasskeys(r passkey.Repo, wa *webauthn.WebAuthn) {
	s.passkeys = r
	s.webauthn = wa
}

// PasskeysEnabled reports whether passkeys are available at all.
func (s *Service) PasskeysEnabled() bool { return s.webauthn != nil && s.passkeys != nil }

func NewService(
	users user.Service,
	tokens magic_token.Repo,
	sessions session.Repo,
	mail mailer.Mailer,
	cfg config.AuthConfig,
	publicURL, frontendURL string,
	log *slog.Logger,
) *Service {
	return &Service{
		users: users, tokens: tokens, sessions: sessions, mail: mail,
		cfg: cfg, publicURL: publicURL, frontendURL: frontendURL, log: log,
		ceremonies: newCeremonyStore(),
		async:      func(f func()) { go f() },
	}
}

// RequestLink provisions an account for email if needed, stores a fresh
// single-use link + 6-digit code bound to browserNonce (the requesting
// browser's cookie), and emails both. It does not reveal whether the address
// was already known.
//
// Every refusal that depends on the address - signup policy, the per-address
// rate limit - returns nil exactly like a sent link, so the response never
// tells a caller whether an account exists or may be created.
func (s *Service) RequestLink(ctx context.Context, email, browserNonce string) error {
	if browserNonce == "" {
		return errors.New("auth: missing browser nonce")
	}
	u, err := s.users.FindByEmail(email)
	switch {
	case errors.Is(err, user.ErrInvalidInput):
		return ErrInvalidEmail
	case errors.Is(err, user.ErrNotFound):
		norm := normalizeAddr(email)
		if !s.signupAllowed(norm) {
			s.log.Info("signup refused by policy", "mode", s.cfg.SignupMode, "domain", domainOf(norm))
			return nil
		}
		u, err = s.users.EnsureByEmail(norm)
		if err != nil {
			return fmt.Errorf("auth: create account: %w", err)
		}
	case err != nil:
		return fmt.Errorf("auth: find account: %w", err)
	}
	if !s.emailLimiter.Allow(u.Email) {
		s.log.Warn("magic link suppressed: per-address limit", "user", u.ID)
		return nil
	}

	raw, err := secret()
	if err != nil {
		return err
	}
	code, err := sixDigits()
	if err != nil {
		return err
	}
	ttl := s.cfg.MagicLinkTTL.Std()
	tokenHash := hash(raw)
	if _, err := s.tokens.Create(&magic_token.MagicToken{
		UserID:      u.ID,
		TokenHash:   tokenHash,
		ExpiresAt:   time.Now().Add(ttl),
		BrowserHash: hash(browserNonce),
		CodeHash:    s.codeMAC(tokenHash, code),
	}); err != nil {
		return fmt.Errorf("auth: store token: %w", err)
	}

	// The link opens the frontend's confirmation page, which shows the
	// account and signs in only on an explicit click (POST) from the browser
	// that asked for it. The token rides in the fragment, so it is never sent
	// to a server, logged, or leaked through a Referer - and a mail scanner
	// that prefetches the URL cannot spend it.
	link := s.ConfirmURL(raw)
	human := emailtmpl.HumanDuration(ttl)
	body := fmt.Sprintf(
		"Use this link to sign in to SADE. It works in the browser where you asked for it, "+
			"is valid for %s and can be used once:\n\n%s\n\n"+
			"Opened this email on another device? Type this code in that browser instead: %s\n\n"+
			"If you did not request this, you can ignore this email.",
		human, link, code,
	)
	html, err := emailtmpl.Render(emailtmpl.Data{
		Subject:     "Sign in to SADE",
		Preheader:   "Your sign-in code is " + code,
		Heading:     "Sign in to SADE",
		Intro:       "Click the button below to sign in. It works in the browser where you asked for the link, is valid for " + human + " and can be used once.",
		Buttons:     []emailtmpl.Button{{Label: "Sign in", URL: link, Primary: true}},
		CodeLabel:   "Reading this on another device? Type this code in the browser where you asked to sign in:",
		Code:        code,
		FallbackURL: link,
		Note:        "If you did not request this, you can ignore this email - nobody can sign in without it.",
	})
	if err != nil {
		s.log.Error("render magic link email", "user", u.ID, "error", err)
		return fmt.Errorf("auth: render email: %w", err)
	}
	if err := s.mail.Send(ctx, mailer.Message{
		To: u.Email, Subject: "Sign in to SADE", Text: body, HTML: html,
		Inline: []mailer.Inline{{CID: emailtmpl.LogoCID, ContentType: "image/png", Data: emailtmpl.LogoPNG}},
	}); err != nil {
		s.log.Error("send magic link", "user", u.ID, "error", err)
		return fmt.Errorf("auth: send link: %w", err)
	}
	s.log.Info("magic link sent", "user", u.ID)
	return nil
}

// ConfirmURL is the frontend sign-in confirmation page for a raw token.
func (s *Service) ConfirmURL(rawToken string) string {
	return s.frontendURL + "/auth/confirm#token=" + url.QueryEscape(rawToken)
}

// Peek returns the email address a still-valid sign-in token belongs to,
// without spending it, and whether browserNonce is the browser that asked
// for it - the confirmation page shows both before the user commits. Any
// unusable token yields ErrInvalidToken.
func (s *Service) Peek(_ context.Context, rawToken, browserNonce string) (email string, sameBrowser bool, err error) {
	if rawToken == "" {
		return "", false, ErrInvalidToken
	}
	mt, err := s.tokens.Peek(hash(rawToken), time.Now())
	if err != nil {
		if errors.Is(err, magic_token.ErrNotFound) || errors.Is(err, magic_token.ErrExpired) {
			return "", false, ErrInvalidToken
		}
		return "", false, fmt.Errorf("auth: peek token: %w", err)
	}
	u, err := s.users.GetByID(mt.UserID)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return "", false, ErrInvalidToken
		}
		return "", false, fmt.Errorf("auth: load token user: %w", err)
	}
	same := browserNonce != "" && mt.BrowserHash != "" && hmac.Equal([]byte(mt.BrowserHash), []byte(hash(browserNonce)))
	return u.Email, same, nil
}

// Complete spends rawToken - only from the browser that requested it - and
// signs in. Any unusable token yields ErrInvalidToken; a valid one opened in
// another browser yields ErrOtherBrowser and stays usable.
func (s *Service) Complete(ctx context.Context, rawToken, browserNonce string, c Client) (SignIn, error) {
	if browserNonce == "" {
		return SignIn{}, ErrOtherBrowser
	}
	userID, err := s.tokens.Consume(hash(rawToken), hash(browserNonce), time.Now())
	switch {
	case errors.Is(err, magic_token.ErrOtherBrowser):
		return SignIn{}, ErrOtherBrowser
	case errors.Is(err, magic_token.ErrNotFound), errors.Is(err, magic_token.ErrExpired):
		return SignIn{}, ErrInvalidToken
	case err != nil:
		return SignIn{}, fmt.Errorf("auth: consume token: %w", err)
	}
	return s.afterEmailStep(ctx, userID, login_event.MethodLink, c)
}

// CompleteWithCode signs in with the 6-digit code from the email, typed in
// the browser that requested it. Wrong code, unknown address, other browser
// and expired email all yield ErrInvalidCode; after maxCodeAttempts wrong
// codes the email is dead.
func (s *Service) CompleteWithCode(ctx context.Context, email, code, browserNonce string, c Client) (SignIn, error) {
	code = strings.TrimSpace(code)
	if !isSixDigits(code) || browserNonce == "" {
		return SignIn{}, ErrInvalidCode
	}
	u, err := s.users.FindByEmail(email)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) || errors.Is(err, user.ErrInvalidInput) {
			return SignIn{}, ErrInvalidCode
		}
		return SignIn{}, fmt.Errorf("auth: find account: %w", err)
	}
	match := func(t magic_token.MagicToken) bool {
		return hmac.Equal([]byte(s.codeMAC(t.TokenHash, code)), []byte(t.CodeHash))
	}
	err = s.tokens.ConsumeByCode(u.ID, hash(browserNonce), match, maxCodeAttempts, time.Now())
	switch {
	case errors.Is(err, magic_token.ErrBadCode), errors.Is(err, magic_token.ErrNotFound):
		s.log.Info("wrong sign-in code", "user", u.ID)
		return SignIn{}, ErrInvalidCode
	case err != nil:
		return SignIn{}, fmt.Errorf("auth: consume code: %w", err)
	}
	return s.afterEmailStep(ctx, u.ID, login_event.MethodCode, c)
}

// afterEmailStep starts a session - or, for an account that requires a
// passkey, hands back the passkey challenge the session now waits on.
func (s *Service) afterEmailStep(ctx context.Context, userID, method string, c Client) (SignIn, error) {
	u, err := s.users.GetByID(userID)
	if err != nil {
		return SignIn{}, fmt.Errorf("auth: load user: %w", err)
	}
	if u.RequirePasskey && s.PasskeysEnabled() {
		ch, err := s.beginSecondFactor(userID)
		if err != nil {
			return SignIn{}, err
		}
		s.log.Info("email step done, passkey required", "user", userID, "method", method)
		return SignIn{Passkey: ch}, nil
	}
	secret, exp, err := s.startSession(ctx, u, method, c)
	if err != nil {
		return SignIn{}, err
	}
	return SignIn{SessionSecret: secret, ExpiresAt: exp}, nil
}

// startSession creates the session, records the sign-in, and alerts the
// owner when it comes from a browser the account has not used recently.
func (s *Service) startSession(ctx context.Context, u user.Response, method string, c Client) (string, time.Time, error) {
	sessionSecret, err := secret()
	if err != nil {
		return "", time.Time{}, err
	}
	now := time.Now()
	expiresAt := now.Add(s.cfg.SessionTTL.Std())
	if _, err := s.sessions.Create(&session.Session{
		UserID:     u.ID,
		TokenHash:  hash(sessionSecret),
		ExpiresAt:  expiresAt,
		UserAgent:  truncate(c.UserAgent, 512),
		IP:         c.IP,
		LastSeenAt: now,
	}); err != nil {
		return "", time.Time{}, fmt.Errorf("auth: create session: %w", err)
	}
	s.recordSignIn(ctx, u, method, c, now)
	s.log.Info("session created", "user", u.ID, "method", method)
	return sessionSecret, expiresAt, nil
}

// Authenticate resolves a session cookie value to its user.
func (s *Service) Authenticate(ctx context.Context, sessionSecret string) (user.Response, error) {
	u, _, err := s.AuthenticateSession(ctx, sessionSecret, "")
	return u, err
}

// AuthenticateSession resolves a session cookie value to its user and the
// session's id, and keeps the session's "last seen" fresh (at most once per
// touchEvery). ip, when set, is recorded as the session's latest address.
func (s *Service) AuthenticateSession(_ context.Context, sessionSecret, ip string) (user.Response, string, error) {
	if sessionSecret == "" {
		return user.Response{}, "", ErrNoSession
	}
	sess, err := s.sessions.GetByHash(hash(sessionSecret))
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return user.Response{}, "", ErrNoSession
		}
		return user.Response{}, "", fmt.Errorf("auth: load session: %w", err)
	}
	now := time.Now()
	if sess.Expired(now) {
		_ = s.sessions.Delete(hash(sessionSecret))
		return user.Response{}, "", ErrSessionExpired
	}
	u, err := s.users.GetByID(sess.UserID)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return user.Response{}, "", ErrNoSession // account was deleted
		}
		return user.Response{}, "", fmt.Errorf("auth: load session user: %w", err)
	}
	if ip != "" && now.Sub(sess.LastSeenAt) > touchEvery {
		if err := s.sessions.Touch(sess.ID, ip, now); err != nil {
			s.log.Warn("touch session", "error", err)
		}
	}
	return u, sess.ID, nil
}

// Logout drops the session for a cookie value. Unknown values are ignored.
func (s *Service) Logout(_ context.Context, sessionSecret string) error {
	if sessionSecret == "" {
		return nil
	}
	return s.sessions.Delete(hash(sessionSecret))
}

// RevokeAll ends every session userID has - "sign out everywhere", the
// alert email's "this wasn't me", and what a role change does so the new
// privileges apply from the next request.
func (s *Service) RevokeAll(_ context.Context, userID string) error {
	if err := s.sessions.DeleteByUser(userID); err != nil {
		return fmt.Errorf("auth: revoke sessions: %w", err)
	}
	s.log.Info("sessions revoked", "user", userID)
	return nil
}

// PurgeExpired deletes sign-in tokens and sessions past their expiry, and
// sign-in history older than the new-device window. app.go runs this
// periodically.
func (s *Service) PurgeExpired(_ context.Context) error {
	now := time.Now()
	tokens, err := s.tokens.DeleteExpired(now)
	if err != nil {
		return err
	}
	sessions, err := s.sessions.DeleteExpired(now)
	if err != nil {
		return err
	}
	events := 0
	if s.events != nil {
		if events, err = s.events.DeleteBefore(now.Add(-knownDeviceWindow)); err != nil {
			return err
		}
	}
	if tokens+sessions+events > 0 {
		s.log.Info("purged expired credentials", "magic_tokens", tokens, "sessions", sessions, "login_events", events)
	}
	return nil
}

// FrontendURL is where the callback redirects a browser after login.
func (s *Service) FrontendURL() string { return s.frontendURL }

// signupAllowed applies Auth.SignupMode to an address with no account yet.
func (s *Service) signupAllowed(email string) bool {
	switch s.cfg.SignupMode {
	case "", "open":
		return true
	case "allowlist":
		for _, e := range s.cfg.SignupAllowedEmails {
			if strings.EqualFold(strings.TrimSpace(e), email) {
				return true
			}
		}
		d := domainOf(email)
		for _, allowed := range s.cfg.SignupAllowedDomains {
			if d != "" && strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(allowed), "@"), d) {
				return true
			}
		}
		return false
	default: // closed
		return false
	}
}

// codeMAC binds a 6-digit code to its email's token and keys it with the
// server secret: a leaked database row cannot be brute-forced offline (a
// plain hash of 6 digits falls in a million guesses).
func (s *Service) codeMAC(tokenHash, code string) string {
	m := hmac.New(sha256.New, []byte(s.cfg.HMACSecret))
	m.Write([]byte("signin-code:" + tokenHash + ":" + code))
	return hex.EncodeToString(m.Sum(nil))
}

func normalizeAddr(email string) string {
	if a, err := mail.ParseAddress(strings.TrimSpace(email)); err == nil {
		return strings.ToLower(a.Address)
	}
	return strings.ToLower(strings.TrimSpace(email))
}

func domainOf(email string) string {
	if i := strings.LastIndexByte(email, '@'); i >= 0 {
		return strings.ToLower(email[i+1:])
	}
	return ""
}

// secret returns 32 bytes of URL-safe base64 randomness.
func secret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: generate secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// sixDigits returns a uniformly random code from 000000 to 999999.
func sixDigits() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("auth: generate code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func isSixDigits(s string) bool {
	if len(s) != 6 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
