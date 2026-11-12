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
	device := deviceRO(DescribeUserAgent(c.UserAgent))
	when := emailtmpl.FormatTime("ro", at) + " (ora României)"
	how := map[string]string{
		login_event.MethodLink:    "link din email",
		login_event.MethodCode:    "cod din email",
		login_event.MethodPasskey: "cheie de acces",
	}[method]
	// The IP address stays in Settings > Security (the sign-in history):
	// a bare number tells a reader nothing they can act on here.

	var buttons []emailtmpl.Button
	revokeURL := ""
	// The reason comes before the button, and says what the button does.
	prompt := "Nu ai fost tu? Deschide Setări › Securitate în SADE și închide sesiunile pe care nu le recunoști."
	if s.signer != nil {
		if tok, err := s.signer.Sign(revokePurpose, u.ID, revokeLinkTTL); err == nil {
			revokeURL = s.frontendURL + "/auth/revoke#token=" + tok
			prompt = "Nu ai fost tu? Închide toate sesiunile: contul se deconectează de pe toate dispozitivele, inclusiv de pe acesta, apoi te conectezi din nou."
			// Ink, not the cheerful orange: it ends every session.
			buttons = []emailtmpl.Button{{Label: "Închide toate sesiunile", URL: revokeURL, Ink: true}}
		}
	}
	intro := "Cineva a intrat în contul tău SADE de pe un dispozitiv pe care nu l-ai mai folosit. Dacă ai fost tu, nu trebuie să faci nimic."
	note := "Cine îți poate citi emailul poate intra și în cont. Adaugă o cheie de acces în Setări › Securitate, unde găsești și adresa IP a fiecărei conectări."
	if revokeURL != "" {
		note = "Linkul butonului e valabil " + emailtmpl.HumanDurationIn("ro", revokeLinkTTL) + ". " + note
	}
	footer := "Îți trimitem acest email ca să-ți protejăm contul SADE."
	rows := []emailtmpl.Row{
		{Label: "Dispozitiv", Value: device},
		{Label: "Ora", Value: when},
		{Label: "Metoda", Value: how},
	}

	var b strings.Builder
	b.WriteString(newDeviceSubject + "\n\n" + intro + "\n\n")
	for _, r := range rows {
		b.WriteString(r.Label + ": " + r.Value + "\n")
	}
	b.WriteString("\n" + prompt + "\n")
	if revokeURL != "" {
		b.WriteString(revokeURL + "\n")
	}
	b.WriteString("\n" + note + "\n\n" + emailtmpl.TextFooter("ro", footer))
	text := b.String()

	html, err := emailtmpl.Render(emailtmpl.Data{
		Lang:        "ro",
		Subject:     newDeviceSubject,
		Preheader:   "Conectare nouă de pe " + device,
		Heading:     "Conectare nouă în cont",
		Intro:       intro,
		Rows:        rows,
		Prompt:      prompt,
		Buttons:     buttons,
		FallbackURL: revokeURL,
		Note:        note,
		Footer:      footer,
	})
	if err != nil {
		s.log.Error("render new-device alert", "user", u.ID, "error", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.mail.Send(ctx, mailer.Message{
		To: u.Email, Subject: newDeviceSubject, Text: text, HTML: html,
		Inline: []mailer.Inline{{CID: emailtmpl.LogoCID, ContentType: "image/png", Data: emailtmpl.LogoPNG}},
	}); err != nil {
		s.log.Error("send new-device alert", "user", u.ID, "error", err)
		return
	}
	s.log.Info("new-device alert sent", "user", u.ID)
}

// newDeviceSubject is the new sign-in alert's subject line.
const newDeviceSubject = "Conectare nouă în contul tău SADE"

// deviceRO is DescribeUserAgent's label in Romanian, for the emails:
// "Chrome on Windows" -> "Chrome pe Windows".
func deviceRO(d string) string {
	switch d {
	case "Unknown device":
		return "Dispozitiv necunoscut"
	}
	d = strings.Replace(d, "SADE app", "Aplicația SADE", 1)
	return strings.Replace(d, " on ", " pe ", 1)
}

// appUAMarker is appended to the user agent of a request from the installed
// app (X-SADE-Client: app), so its sessions and sign-ins name the app.
const appUAMarker = "SADEApp"

// DescribeUserAgent turns a User-Agent header into "Chrome on Windows". It
// only needs to be good enough for a person to recognise their own device.
func DescribeUserAgent(ua string) string {
	if ua == "" {
		return "Unknown device"
	}
	browser := "Browser"
	switch {
	case strings.Contains(ua, appUAMarker):
		browser = "SADE app"
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
