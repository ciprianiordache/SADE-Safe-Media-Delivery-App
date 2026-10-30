package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sade/config"
	"sade/internals/testutil"
)

func newHandler(t *testing.T) (*Handler, *captureMailer) {
	svc, cm := newService(t)
	return NewHandler(svc, config.AuthConfig{SessionCookieName: "sade_session"}, testutil.Logger()), cm
}

func TestHandlerRequestLink(t *testing.T) {
	h, cm := newHandler(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/auth/request", strings.NewReader(`{"email":"a@b.c"}`))
	h.RequestLink(w, r)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d (%s)", w.Code, w.Body.String())
	}
	if len(cm.sent) != 1 {
		t.Fatalf("expected 1 email, got %d", len(cm.sent))
	}

	// bad email -> 400
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/auth/request", strings.NewReader(`{"email":"nope"}`))
	h.RequestLink(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("bad email status = %d, want 400", w.Code)
	}
}

func TestHandlerCallbackForwardsToConfirmWithoutSigningIn(t *testing.T) {
	h, cm := newHandler(t)
	if err := h.svc.RequestLink(context.Background(), "a@b.c"); err != nil {
		t.Fatal(err)
	}
	tok := linkToken(t, cm)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/auth/callback?token="+tok, nil)
	h.Callback(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "http://app.test/auth/confirm#token="+tok {
		t.Errorf("Location = %q", loc)
	}
	if len(w.Result().Cookies()) != 0 {
		t.Errorf("a GET must never sign anyone in")
	}
	// The token is still usable: the GET did not spend it.
	if _, err := h.svc.Peek(context.Background(), tok); err != nil {
		t.Errorf("token spent by GET callback: %v", err)
	}
}

func TestHandlerVerifyThenComplete(t *testing.T) {
	h, cm := newHandler(t)
	if err := h.svc.RequestLink(context.Background(), "a@b.c"); err != nil {
		t.Fatal(err)
	}
	body := `{"token":"` + linkToken(t, cm) + `"}`

	w := httptest.NewRecorder()
	h.Verify(w, httptest.NewRequest(http.MethodPost, "/api/auth/verify", strings.NewReader(body)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "a@b.c") {
		t.Fatalf("verify = %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.Complete(w, httptest.NewRequest(http.MethodPost, "/api/auth/complete", strings.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("complete = %d %s", w.Code, w.Body.String())
	}
	var sc *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "sade_session" {
			sc = c
		}
	}
	if sc == nil || sc.Value == "" || !sc.HttpOnly {
		t.Fatalf("session cookie missing or not HttpOnly: %+v", sc)
	}

	// Single use.
	w = httptest.NewRecorder()
	h.Complete(w, httptest.NewRequest(http.MethodPost, "/api/auth/complete", strings.NewReader(body)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("second complete = %d, want 400", w.Code)
	}
}

func TestHandlerCallbackBadTokenRedirectsToLogin(t *testing.T) {
	h, _ := newHandler(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/auth/callback?token=garbage", nil)
	h.Callback(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", w.Code)
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "error=invalid_link") {
		t.Errorf("Location = %q, want the login page with an error", loc)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "sade_session" && c.Value != "" {
			t.Errorf("bad callback should not set a session cookie")
		}
	}
}

func TestHandlerLogoutClearsCookie(t *testing.T) {
	h, cm := newHandler(t)
	_ = h.svc.RequestLink(context.Background(), "a@b.c")
	secret, _, _ := h.svc.Complete(context.Background(), linkToken(t, cm))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	r.AddCookie(&http.Cookie{Name: "sade_session", Value: secret})
	h.Logout(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	var cleared bool
	for _, c := range w.Result().Cookies() {
		if c.Name == "sade_session" && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("logout did not clear the session cookie")
	}
	if _, err := h.svc.Authenticate(context.Background(), secret); err == nil {
		t.Error("session still valid after logout")
	}
}
