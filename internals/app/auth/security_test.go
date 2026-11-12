package auth

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"

	"sade/internals/app/login_event"
	"sade/internals/app/passkey"
	"sade/internals/token"

	"github.com/go-webauthn/webauthn/webauthn"
)

var codeRe = regexp.MustCompile(`conectarea: (\d{6})`)

func emailCode(t *testing.T, cm *captureMailer) string {
	t.Helper()
	m := codeRe.FindStringSubmatch(cm.last().Text)
	if m == nil {
		t.Fatalf("no code in email:\n%s", cm.last().Text)
	}
	return m[1]
}

var (
	chromeWin = Client{UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/130.0 Safari/537.36", IP: "203.0.113.5"}
	safariIOS = Client{UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 Version/18.0 Mobile/15E148 Safari/604.1", IP: "198.51.100.9"}
)

func TestCodeSignsInOnlyFromTheRequestingBrowser(t *testing.T) {
	svc, cm := newService(t)
	ctx := context.Background()
	if err := svc.RequestLink(ctx, "op@example.com", testNonce); err != nil {
		t.Fatal(err)
	}
	code := emailCode(t, cm)

	if _, err := svc.CompleteWithCode(ctx, "op@example.com", code, "another-browser-nonce", chromeWin); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("code from another browser = %v, want ErrInvalidCode", err)
	}
	if _, err := svc.CompleteWithCode(ctx, "nobody@example.com", code, testNonce, chromeWin); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("code for another address = %v, want ErrInvalidCode", err)
	}
	res, err := svc.CompleteWithCode(ctx, "OP@example.com", code, testNonce, chromeWin)
	if err != nil || res.SessionSecret == "" {
		t.Fatalf("right code = %+v, %v", res, err)
	}
	// Spent: neither the code nor the link works again.
	if _, err := svc.CompleteWithCode(ctx, "op@example.com", code, testNonce, chromeWin); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("reused code = %v", err)
	}
	if _, _, err := completeT(svc, ctx, linkToken(t, cm)); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("link after code sign-in = %v, want ErrInvalidToken", err)
	}
}

func TestWrongCodesKillTheEmail(t *testing.T) {
	svc, cm := newService(t)
	ctx := context.Background()
	_ = svc.RequestLink(ctx, "op@example.com", testNonce)
	code := emailCode(t, cm)
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	for i := 0; i < maxCodeAttempts; i++ {
		if _, err := svc.CompleteWithCode(ctx, "op@example.com", wrong, testNonce, chromeWin); !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("wrong code #%d = %v", i+1, err)
		}
	}
	if _, err := svc.CompleteWithCode(ctx, "op@example.com", code, testNonce, chromeWin); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("right code after lockout = %v, want ErrInvalidCode", err)
	}
}

func TestLinkFromAnotherBrowserIsRefusedAndKept(t *testing.T) {
	svc, cm := newService(t)
	ctx := context.Background()
	_ = svc.RequestLink(ctx, "op@example.com", testNonce)
	tok := linkToken(t, cm)

	if _, same, _ := svc.Peek(ctx, tok, "other-browser-nonce-xx"); same {
		t.Error("Peek says same browser for a different cookie")
	}
	if _, err := svc.Complete(ctx, tok, "other-browser-nonce-xx", chromeWin); !errors.Is(err, ErrOtherBrowser) {
		t.Fatalf("Complete(other browser) = %v, want ErrOtherBrowser", err)
	}
	if _, _, err := completeT(svc, ctx, tok); err != nil {
		t.Fatalf("link no longer works in its own browser: %v", err)
	}
}

func TestNewDeviceAlert(t *testing.T) {
	svc, cm := newService(t)
	signer, err := token.New(strings.Repeat("k", 32))
	if err != nil {
		t.Fatal(err)
	}
	svc.UseAlertSigner(signer)
	ctx := context.Background()

	signIn := func(c Client) {
		t.Helper()
		if err := svc.RequestLink(ctx, "op@example.com", testNonce); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Complete(ctx, linkToken(t, cm), testNonce, c); err != nil {
			t.Fatal(err)
		}
	}
	alerts := func() int {
		n := 0
		for _, m := range cm.sent {
			if m.Subject == newDeviceSubject {
				n++
			}
		}
		return n
	}

	signIn(chromeWin) // first sign-in ever: nothing to compare against
	signIn(chromeWin) // same browser again
	if n := alerts(); n != 0 {
		t.Fatalf("alerts after known-browser sign-ins = %d, want 0", n)
	}
	signIn(safariIOS)
	if n := alerts(); n != 1 {
		t.Fatalf("alerts after a new device = %d, want 1", n)
	}
	alert := cm.last()
	// The device and the reason before the button are in it; the bare IP
	// stays in Settings > Security, next to the sign-in history.
	if !strings.Contains(alert.Text, "Safari pe iPhone") || !strings.Contains(alert.Text, "Nu ai fost tu?") ||
		strings.Contains(alert.Text, "198.51.100.9") {
		t.Errorf("alert text:\n%s", alert.Text)
	}
	if p, b := strings.Index(alert.HTML, "Nu ai fost tu?"), strings.Index(alert.HTML, "v:roundrect"); p < 0 || b < 0 || p > b {
		t.Errorf("the reason must come before the button")
	}

	// Its "this wasn't me" link ends every session.
	i := strings.Index(alert.Text, "/auth/revoke#token=")
	if i < 0 {
		t.Fatalf("no revoke link in alert:\n%s", alert.Text)
	}
	tok := strings.Fields(alert.Text[i+len("/auth/revoke#token="):])[0]
	email, err := svc.RevokeFromAlert(ctx, tok)
	if err != nil || email != "op@example.com" {
		t.Fatalf("RevokeFromAlert = %q, %v", email, err)
	}
	u, _ := svc.users.FindByEmail("op@example.com")
	if left, _ := svc.ListSessions(ctx, u.ID, ""); len(left) != 0 {
		t.Errorf("%d sessions left after revoke", len(left))
	}
	if _, err := svc.RevokeFromAlert(ctx, "forged.token"); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("forged revoke token = %v", err)
	}

	acts, err := svc.Activity(ctx, u.ID)
	if err != nil || len(acts) != 3 {
		t.Fatalf("activity = %+v, %v", acts, err)
	}
	devices := map[string]int{}
	for _, a := range acts {
		if a.Method != login_event.MethodLink {
			t.Errorf("method = %q, want link", a.Method)
		}
		devices[a.Device]++
	}
	if devices["Chrome on Windows"] != 2 || devices["Safari on iPhone"] != 1 {
		t.Errorf("activity devices = %v", devices)
	}
}

func TestListAndRevokeSessions(t *testing.T) {
	svc, cm := newService(t)
	ctx := context.Background()
	var ids []string
	for _, c := range []Client{chromeWin, safariIOS} {
		_ = svc.RequestLink(ctx, "op@example.com", testNonce)
		res, err := svc.Complete(ctx, linkToken(t, cm), testNonce, c)
		if err != nil {
			t.Fatal(err)
		}
		_, sid, err := svc.AuthenticateSession(ctx, res.SessionSecret, "")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, sid)
	}
	u, _ := svc.users.FindByEmail("op@example.com")

	list, err := svc.ListSessions(ctx, u.ID, ids[0])
	if err != nil || len(list) != 2 {
		t.Fatalf("ListSessions = %+v, %v", list, err)
	}
	for _, s := range list {
		if (s.ID == ids[0]) != s.Current {
			t.Errorf("current flag wrong on %+v", s)
		}
	}
	if err := svc.RevokeSession(ctx, "someone-else", ids[1]); !errors.Is(err, ErrNotFound) {
		t.Errorf("revoking another account's session = %v, want ErrNotFound", err)
	}
	if err := svc.RevokeSession(ctx, u.ID, ids[1]); err != nil {
		t.Fatal(err)
	}
	if list, _ := svc.ListSessions(ctx, u.ID, ids[0]); len(list) != 1 {
		t.Errorf("sessions after revoke = %d, want 1", len(list))
	}
}

func TestRequirePasskey(t *testing.T) {
	svc, cm := newService(t)
	ctx := context.Background()
	_ = svc.RequestLink(ctx, "op@example.com", testNonce)
	if _, _, err := completeT(svc, ctx, linkToken(t, cm)); err != nil {
		t.Fatal(err)
	}
	u, _ := svc.users.FindByEmail("op@example.com")

	wa, err := webauthn.New(&webauthn.Config{RPID: "app.test", RPDisplayName: "SADE", RPOrigins: []string{"http://app.test"}})
	if err != nil {
		t.Fatal(err)
	}
	svc.UsePasskeys(svc.passkeys, wa)

	if _, err := svc.SetRequirePasskey(ctx, u.ID, true); !errors.Is(err, ErrNoPasskey) {
		t.Fatalf("require without a passkey = %v, want ErrNoPasskey", err)
	}

	cred, _ := json.Marshal(webauthn.Credential{ID: []byte("cred-1"), PublicKey: []byte("pk")})
	pkID, err := svc.passkeys.Create(&passkey.Passkey{UserID: u.ID, CredentialID: credID([]byte("cred-1")), Credential: string(cred), Name: "Laptop"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.SetRequirePasskey(ctx, u.ID, true)
	if err != nil || !got.RequirePasskey {
		t.Fatalf("require with a passkey = %+v, %v", got, err)
	}

	// The email step alone no longer starts a session.
	_ = svc.RequestLink(ctx, "op@example.com", testNonce)
	res, err := svc.Complete(ctx, linkToken(t, cm), testNonce, chromeWin)
	if err != nil {
		t.Fatal(err)
	}
	if res.SessionSecret != "" || res.Passkey == nil || res.Passkey.CeremonyID == "" {
		t.Fatalf("email step with passkey required = %+v", res)
	}
	// A made-up assertion is refused, and the ceremony is single-use.
	if _, _, err := svc.FinishPasskeyLogin(ctx, res.Passkey.CeremonyID, []byte(`{}`), chromeWin); !errors.Is(err, ErrInvalidPasskey) {
		t.Errorf("bogus assertion = %v, want ErrInvalidPasskey", err)
	}

	// Removing the last passkey turns the requirement off (no lock-out).
	if err := svc.DeletePasskey(ctx, u.ID, pkID); err != nil {
		t.Fatal(err)
	}
	after, _ := svc.users.GetByID(u.ID)
	if after.RequirePasskey {
		t.Error("requirement still on with no passkey left")
	}
}

func TestDescribeUserAgent(t *testing.T) {
	cases := map[string]string{
		chromeWin.UserAgent: "Chrome on Windows",
		safariIOS.UserAgent: "Safari on iPhone",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) Gecko/20100101 Firefox/131.0":              "Firefox on macOS",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/130.0 Safari/537.36 Edg/130.0":         "Edge on Windows",
		"Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 Chrome/130.0 Mobile Safari/537.36":   "Chrome on Android",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/130.0 Safari/537.36 Edg/130.0 SADEApp": "SADE app on Windows",
		"": "Unknown device",
	}
	for ua, want := range cases {
		if got := DescribeUserAgent(ua); got != want {
			t.Errorf("DescribeUserAgent(%q) = %q, want %q", ua, got, want)
		}
	}
}
