package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"sade/config"
	"sade/internals/app/auth"
	"sade/internals/app/magic_token"
	"sade/internals/app/session"
	"sade/internals/app/user"
	"sade/internals/mailer"
	"sade/internals/testutil"
)

type capMailer struct {
	mu   sync.Mutex
	sent []mailer.Message
}

func (m *capMailer) Send(_ context.Context, msg mailer.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}

func (m *capMailer) lastLink(t *testing.T) string {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	body := m.sent[len(m.sent)-1].Text
	i := strings.Index(body, "http://")
	if i < 0 {
		t.Fatalf("no link in body:\n%s", body)
	}
	link := body[i:]
	if j := strings.IndexAny(link, " \n\r"); j >= 0 {
		link = link[:j]
	}
	return link
}

// buildTestApp wires the real router over an in-memory DB and a capturing
// mailer, and returns an httptest server plus a client with a cookie jar.
func buildTestApp(t *testing.T) (*httptest.Server, *http.Client, *capMailer) {
	t.Helper()
	db := testutil.DB(t, user.User{}, magic_token.MagicToken{}, session.Session{})
	cm := &capMailer{}

	cfg := &config.Config{}
	cfg.Server.CORSAllowedOrigins = []string{"http://localhost:5173"}
	cfg.Auth = config.AuthConfig{
		MagicLinkTTL:      config.Duration(15 * time.Minute),
		SessionTTL:        config.Duration(24 * time.Hour),
		SessionCookieName: "sade_session",
	}

	userSvc := user.NewService(user.NewRepo(db), testutil.Logger())
	authSvc := auth.NewService(
		userSvc, magic_token.NewRepo(db), session.NewRepo(db), cm,
		cfg.Auth, "http://APIBASE", "http://app.test", testutil.Logger(),
	)

	router := NewRouter(Deps{
		Cfg:     cfg,
		Log:     testutil.Logger(),
		Auth:    auth.NewHandler(authSvc, cfg.Auth, testutil.Logger()),
		AuthSvc: authSvc,
		User:    user.NewHandler(userSvc, testutil.Logger()),
	})

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		// Do not follow the callback's redirect - we want to inspect it.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return srv, client, cm
}

func TestAuthFlowEndToEnd(t *testing.T) {
	srv, client, cm := buildTestApp(t)

	// 0. /api/me while signed out -> 401
	if resp := get(t, client, srv.URL+"/api/me"); resp != http.StatusUnauthorized {
		t.Fatalf("/api/me signed out = %d, want 401", resp)
	}

	// 1. request a link
	resp, err := client.Post(srv.URL+"/api/auth/request", "application/json",
		strings.NewReader(`{"email":"op@example.com"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("request link = %d, want 202", resp.StatusCode)
	}

	// 2. follow the callback link from the email; the API base in the link
	// is a placeholder, so swap in the test server's URL.
	link := cm.lastLink(t)
	link = strings.Replace(link, "http://APIBASE", srv.URL, 1)
	cb, err := client.Get(link)
	if err != nil {
		t.Fatal(err)
	}
	cb.Body.Close()
	if cb.StatusCode != http.StatusSeeOther {
		t.Fatalf("callback = %d, want 303", cb.StatusCode)
	}
	if loc := cb.Header.Get("Location"); loc != "http://app.test" {
		t.Errorf("callback Location = %q", loc)
	}
	if !hasSessionCookie(client, srv.URL) {
		t.Fatal("no session cookie in the jar after callback")
	}

	// 3. /api/me now returns the signed-in user
	me, err := client.Get(srv.URL + "/api/me")
	if err != nil {
		t.Fatal(err)
	}
	defer me.Body.Close()
	if me.StatusCode != http.StatusOK {
		t.Fatalf("/api/me signed in = %d, want 200", me.StatusCode)
	}
	var u user.Response
	if err := json.NewDecoder(me.Body).Decode(&u); err != nil {
		t.Fatal(err)
	}
	if u.Email != "op@example.com" || u.Role != user.RoleOperator {
		t.Errorf("/api/me user = %+v", u)
	}

	// 4. an operator cannot reach the admin route
	if resp := get(t, client, srv.URL+"/api/users"); resp != http.StatusForbidden {
		t.Errorf("/api/users as operator = %d, want 403", resp)
	}

	// 5. logout, then /api/me is 401 again
	lo, err := client.Post(srv.URL+"/api/auth/logout", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	lo.Body.Close()
	if lo.StatusCode != http.StatusNoContent {
		t.Fatalf("logout = %d, want 204", lo.StatusCode)
	}
	if resp := get(t, client, srv.URL+"/api/me"); resp != http.StatusUnauthorized {
		t.Errorf("/api/me after logout = %d, want 401", resp)
	}
}

func TestHealthz(t *testing.T) {
	srv, client, _ := buildTestApp(t)
	resp, err := client.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/healthz = %d", resp.StatusCode)
	}
}

func TestCORSPreflight(t *testing.T) {
	srv, client, _ := buildTestApp(t)
	req, _ := http.NewRequest(http.MethodOptions, srv.URL+"/api/me", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight = %d, want 204", resp.StatusCode)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Errorf("missing CORS allow-origin: %v", resp.Header)
	}
	if resp.Header.Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("missing allow-credentials")
	}
}

func get(t *testing.T, c *http.Client, url string) int {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func hasSessionCookie(c *http.Client, base string) bool {
	u, _ := http.NewRequest("GET", base, nil)
	for _, ck := range c.Jar.Cookies(u.URL) {
		if ck.Name == "sade_session" && ck.Value != "" {
			return true
		}
	}
	return false
}
