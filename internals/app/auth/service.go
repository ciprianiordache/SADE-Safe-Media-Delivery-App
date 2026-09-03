// Package auth is the magic-link login flow. It has no table of its own: it
// composes the user, magic_token and session domains plus the mailer.
//
//	RequestLink  -> ensure an account, mint a token, email the callback URL
//	Complete     -> spend the token, create a session, return its secret
//	Authenticate -> resolve a session secret to its user
//	Logout       -> drop the session
//
// Only hashes of the magic-link token and the session secret are stored, so a
// database leak yields neither working links nor live sessions.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"sade/config"
	"sade/internals/app/magic_token"
	"sade/internals/app/session"
	"sade/internals/app/user"
	"sade/internals/mailer"
)

// Service runs the login flow.
type Service struct {
	users       user.Service
	tokens      magic_token.Repo
	sessions    session.Repo
	mail        mailer.Mailer
	cfg         config.AuthConfig
	publicURL   string // API base for the callback link (config.App.PublicURL)
	frontendURL string
	log         *slog.Logger
}

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
	}
}

// RequestLink provisions an account for email if needed, stores a fresh
// single-use token, and emails its callback URL. It does not reveal whether
// the address was already known.
func (s *Service) RequestLink(ctx context.Context, email string) error {
	u, err := s.users.EnsureByEmail(email)
	if err != nil {
		if errors.Is(err, user.ErrInvalidInput) {
			return ErrInvalidEmail
		}
		return fmt.Errorf("auth: ensure account: %w", err)
	}

	raw, err := secret()
	if err != nil {
		return err
	}
	ttl := s.cfg.MagicLinkTTL.Std()
	if _, err := s.tokens.Create(&magic_token.MagicToken{
		UserID:    u.ID,
		TokenHash: hash(raw),
		ExpiresAt: time.Now().Add(ttl),
	}); err != nil {
		return fmt.Errorf("auth: store token: %w", err)
	}

	link := s.publicURL + "/api/auth/callback?token=" + url.QueryEscape(raw)
	body := fmt.Sprintf(
		"Use this link to sign in to SADE. It is valid for %s and can be used once:\n\n%s\n\n"+
			"If you did not request this, you can ignore this email.",
		ttl, link,
	)
	if err := s.mail.Send(ctx, mailer.Message{
		To: u.Email, Subject: "Sign in to SADE", Text: body,
	}); err != nil {
		s.log.Error("send magic link", "user", u.ID, "error", err)
		return fmt.Errorf("auth: send link: %w", err)
	}
	s.log.Info("magic link sent", "user", u.ID)
	return nil
}

// Complete spends rawToken and returns a new session's secret and expiry.
// Any unusable token yields ErrInvalidToken.
func (s *Service) Complete(_ context.Context, rawToken string) (sessionSecret string, expiresAt time.Time, err error) {
	userID, err := s.tokens.Consume(hash(rawToken), time.Now())
	if err != nil {
		if errors.Is(err, magic_token.ErrNotFound) || errors.Is(err, magic_token.ErrExpired) {
			return "", time.Time{}, ErrInvalidToken
		}
		return "", time.Time{}, fmt.Errorf("auth: consume token: %w", err)
	}

	sessionSecret, err = secret()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt = time.Now().Add(s.cfg.SessionTTL.Std())
	if _, err := s.sessions.Create(&session.Session{
		UserID:    userID,
		TokenHash: hash(sessionSecret),
		ExpiresAt: expiresAt,
	}); err != nil {
		return "", time.Time{}, fmt.Errorf("auth: create session: %w", err)
	}
	s.log.Info("session created", "user", userID)
	return sessionSecret, expiresAt, nil
}

// Authenticate resolves a session cookie value to its user.
func (s *Service) Authenticate(_ context.Context, sessionSecret string) (user.Response, error) {
	if sessionSecret == "" {
		return user.Response{}, ErrNoSession
	}
	sess, err := s.sessions.GetByHash(hash(sessionSecret))
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return user.Response{}, ErrNoSession
		}
		return user.Response{}, fmt.Errorf("auth: load session: %w", err)
	}
	if sess.Expired(time.Now()) {
		_ = s.sessions.Delete(hash(sessionSecret))
		return user.Response{}, ErrSessionExpired
	}
	u, err := s.users.GetByID(sess.UserID)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return user.Response{}, ErrNoSession // account was deleted
		}
		return user.Response{}, fmt.Errorf("auth: load session user: %w", err)
	}
	return u, nil
}

// Logout drops the session for a cookie value. Unknown values are ignored.
func (s *Service) Logout(_ context.Context, sessionSecret string) error {
	if sessionSecret == "" {
		return nil
	}
	return s.sessions.Delete(hash(sessionSecret))
}

// FrontendURL is where the callback redirects a browser after login.
func (s *Service) FrontendURL() string { return s.frontendURL }

// secret returns 32 bytes of URL-safe base64 randomness.
func secret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: generate secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
