package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"sade/internals/app/login_event"
	"sade/internals/app/session"
	"sade/internals/app/user"
	"sade/internals/emailtmpl"
	"sade/internals/mailer"
)

// knownDeviceWindow is how long a browser counts as known: a sign-in from a
// user agent the account has not used within it earns an alert email. Also
// how long sign-in history is kept.
const knownDeviceWindow = 90 * 24 * time.Hour

// revokePurpose is the token purpose of the alert email's "this wasn't me"
// link. Domain-separated from the share-link purposes by the signer.
const revokePurpose = "revoke-sessions"

// revokeLinkTTL bounds that link: long enough to notice the email.
const revokeLinkTTL = 7 * 24 * time.Hour

// SessionView is one row of the account's "Devices" list.
type SessionView struct {
	ID         string    `json:"id"`
	Device     string    `json:"device"`
	IP         string    `json:"ip"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
	Current    bool      `json:"current"`
}

// EventView is one row of the account's "Recent activity" list.
type EventView struct {
	Method    string    `json:"method"`
	Device    string    `json:"device"`
	IP        string    `json:"ip"`
	CreatedAt time.Time `json:"createdAt"`
}

// ListSessions returns userID's live sessions, marking currentID.
func (s *Service) ListSessions(_ context.Context, userID, currentID string) ([]SessionView, error) {
	rows, err := s.sessions.ListByUser(userID)
	if err != nil {
		return nil, fmt.Errorf("auth: list sessions: %w", err)
	}
	now := time.Now()
	out := make([]SessionView, 0, len(rows))
	for _, r := range rows {
		if r.Expired(now) {
			continue
		}
		seen := r.LastSeenAt
		if seen.IsZero() {
			seen = r.CreatedAt
		}
		out = append(out, SessionView{
			ID: r.ID, Device: DescribeUserAgent(r.UserAgent), IP: r.IP,
			CreatedAt: r.CreatedAt, LastSeenAt: seen, Current: r.ID == currentID,
		})
	}
	return out, nil
}

// RevokeSession ends one of userID's sessions.
func (s *Service) RevokeSession(_ context.Context, userID, sessionID string) error {
	err := s.sessions.DeleteByID(userID, sessionID)
	if errors.Is(err, session.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("auth: revoke session: %w", err)
	}
	s.log.Info("session revoked", "user", userID, "session", sessionID)
	return nil
}

// Activity returns userID's recent sign-ins, newest first.
func (s *Service) Activity(_ context.Context, userID string) ([]EventView, error) {
	if s.events == nil {
		return []EventView{}, nil
	}
	rows, err := s.events.Recent(userID, 20)
	if err != nil {
		return nil, err
	}
	out := make([]EventView, 0, len(rows))
	for _, e := range rows {
		out = append(out, EventView{Method: e.Method, Device: DescribeUserAgent(e.UserAgent), IP: e.IP, CreatedAt: e.CreatedAt})
	}
	return out, nil
}

// RevokeFromAlert handles the alert email's "this wasn't me" link: it ends
// every session of the account the signed token names. Returns the account
// email for the confirmation page.
func (s *Service) RevokeFromAlert(ctx context.Context, rawToken string) (string, error) {
	if s.signer == nil {
		return "", ErrInvalidToken
	}
	userID, err := s.signer.Verify(revokePurpose, rawToken)
	if err != nil {
		return "", ErrInvalidToken
	}
	u, err := s.users.GetByID(userID)
	if err != nil {
		return "", ErrInvalidToken
	}
	if err := s.RevokeAll(ctx, userID); err != nil {
		return "", err
	}
	s.log.Warn("sessions revoked from new-device alert", "user", userID)
	return u.Email, nil
}

// recordSignIn stores the sign-in and, when it comes from a browser the
// account has not signed in from within knownDeviceWindow, emails the owner.
// The very first sign-in of an account is not "new" - there is nothing to
// compare against. Failures here are logged, never fatal to the sign-in.
func (s *Service) recordSignIn(_ context.Context, u user.Response, method string, c Client, now time.Time) {
	if s.events == nil {
		return
	}
	ua := truncate(c.UserAgent, 512)
	seen, err := s.events.SeenUserAgent(u.ID, ua, now.Add(-knownDeviceWindow))
	if err != nil {
		s.log.Warn("check known device", "user", u.ID, "error", err)
		seen = true // don't alert on a lookup failure
	}
	first := false
	if !seen {
		recent, err := s.events.Recent(u.ID, 1)
		first = err == nil && len(recent) == 0
	}
	if err := s.events.Create(&login_event.LoginEvent{UserID: u.ID, Method: method, UserAgent: ua, IP: c.IP}); err != nil {
		s.log.Warn("record sign-in", "user", u.ID, "error", err)
	}
	if seen || first {
		return
	}
	s.async(func() { s.sendNewDeviceAlert(u, method, c, now) })
}

func (s *Service) sendNewDeviceAlert(u user.Response, method string, c Client, at time.Time) {
	device := DescribeUserAgent(c.UserAgent)
	when := at.UTC().Format("2 Jan 2006, 15:04 UTC")
	how := map[string]string{
		login_event.MethodLink:    "an emailed link",
		login_event.MethodCode:    "an emailed code",
		login_event.MethodPasskey: "a passkey",
	}[method]

	var buttons []emailtmpl.Button
	revokeURL := ""
	if s.signer != nil {
		if tok, err := s.signer.Sign(revokePurpose, u.ID, revokeLinkTTL); err == nil {
			revokeURL = s.frontendURL + "/auth/revoke#token=" + tok
			buttons = []emailtmpl.Button{{Label: "This wasn't me - sign out everywhere", URL: revokeURL, Primary: true}}
		}
	}
	text := fmt.Sprintf(
		"Someone signed in to your SADE account from a new device.\n\n"+
			"Device: %s\nIP address: %s\nTime: %s\nMethod: %s\n\n"+
			"If this was you, there is nothing to do.\n",
		device, c.IP, when, how)
	if revokeURL != "" {
		text += "If it wasn't, end every session now:\n" + revokeURL + "\n"
	}
	html, err := emailtmpl.Render(emailtmpl.Data{
		Subject:   "New sign-in to your SADE account",
		Preheader: "New sign-in from " + device,
		Heading:   "New sign-in to your account",
		Intro: fmt.Sprintf("Someone signed in from a device this account hasn't used before: %s, IP address %s, at %s, using %s. If this was you, there is nothing to do.",
			device, c.IP, when, how),
		Buttons: buttons,
		Note:    "If it wasn't you, use the button to end every session, then sign in again - and consider adding a passkey in Settings, so an email alone can't open your account.",
	})
	if err != nil {
		s.log.Error("render new-device alert", "user", u.ID, "error", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.mail.Send(ctx, mailer.Message{
		To: u.Email, Subject: "New sign-in to your SADE account", Text: text, HTML: html,
		Inline: []mailer.Inline{{CID: emailtmpl.LogoCID, ContentType: "image/png", Data: emailtmpl.LogoPNG}},
	}); err != nil {
		s.log.Error("send new-device alert", "user", u.ID, "error", err)
		return
	}
	s.log.Info("new-device alert sent", "user", u.ID)
}

// DescribeUserAgent turns a User-Agent header into "Chrome on Windows". It
// only needs to be good enough for a person to recognise their own device.
func DescribeUserAgent(ua string) string {
	if ua == "" {
		return "Unknown device"
	}
	browser := "Browser"
	switch {
	case strings.Contains(ua, "Edg/"):
		browser = "Edge"
	case strings.Contains(ua, "OPR/"), strings.Contains(ua, "Opera"):
		browser = "Opera"
	case strings.Contains(ua, "SamsungBrowser/"):
		browser = "Samsung Internet"
	case strings.Contains(ua, "Firefox/"), strings.Contains(ua, "FxiOS/"):
		browser = "Firefox"
	case strings.Contains(ua, "Chrome/"), strings.Contains(ua, "CriOS/"):
		browser = "Chrome"
	case strings.Contains(ua, "Safari/"):
		browser = "Safari"
	case strings.Contains(ua, "curl/"):
		return "curl"
	}
	os := ""
	switch {
	case strings.Contains(ua, "iPhone"):
		os = "iPhone"
	case strings.Contains(ua, "iPad"):
		os = "iPad"
	case strings.Contains(ua, "Android"):
		os = "Android"
	case strings.Contains(ua, "Windows"):
		os = "Windows"
	case strings.Contains(ua, "Mac OS X"), strings.Contains(ua, "Macintosh"):
		os = "macOS"
	case strings.Contains(ua, "CrOS"):
		os = "ChromeOS"
	case strings.Contains(ua, "Linux"):
		os = "Linux"
	}
	if os == "" {
		return browser
	}
	return browser + " on " + os
}
