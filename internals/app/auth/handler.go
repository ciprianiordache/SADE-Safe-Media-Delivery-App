package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"sade/config"
	"sade/internals/httpx"
)

// browserCookie names the cookie that ties a sign-in email to the browser
// that asked for it. Only the sign-in endpoints need it, hence the path.
const browserCookie = "sade_login"

// The installed app cannot keep a cookie for the API (it runs on its own
// origin), so it sends the same binding value as a header it generated and
// keeps itself, and asks for the session secret back as a bearer token.
const (
	appClientHeader = "X-SADE-Client" // "app"
	appLoginHeader  = "X-SADE-Login"  // the app's binding nonce
)

func isApp(r *http.Request) bool { return r.Header.Get(appClientHeader) == "app" }

// Handler is the HTTP surface of the sign-in flow and of the account's
// security settings (devices, activity, passkeys). The current-user endpoint
// (/api/me) lives in the router layer. Routes are registered in
// internals/app/router.go.
type Handler struct {
	svc *Service
	cfg config.AuthConfig
	log *slog.Logger

	// clientIP resolves the real client address (proxy-aware); set by the
	// router, which owns the proxy-trust config. Falls back to RemoteAddr.
	clientIP func(*http.Request) string
}

func NewHandler(svc *Service, cfg config.AuthConfig, log *slog.Logger) *Handler {
	return &Handler{svc: svc, cfg: cfg, log: log}
}

// SetClientIP installs the router's proxy-aware client address resolver.
func (h *Handler) SetClientIP(f func(*http.Request) string) { h.clientIP = f }

func (h *Handler) client(r *http.Request) Client {
	ip := r.RemoteAddr
	if h.clientIP != nil {
		ip = h.clientIP(r)
	}
	ua := r.UserAgent()
	if isApp(r) {
		// The installed app runs in the system's web view, whose user agent
		// names a browser (WebView2 says "Edg/"). Mark it so the account's
		// device list says "SADE app on Windows", not a browser it never opened.
		ua += " " + appUAMarker
	}
	return Client{UserAgent: ua, IP: ip}
}

// --- who is calling (set by the router's Auth middleware) -------------------

type callerKey struct{}

type caller struct{ userID, sessionID string }

// WithCaller attaches the signed-in user's id and their session's id; the
// router's Auth middleware calls it, the account handlers below read it.
func WithCaller(ctx context.Context, userID, sessionID string) context.Context {
	return context.WithValue(ctx, callerKey{}, caller{userID, sessionID})
}

func callerFrom(ctx context.Context) (caller, bool) {
	c, ok := ctx.Value(callerKey{}).(caller)
	return c, ok && c.userID != ""
}

// --- sign-in ------------------------------------------------------------------

// Config handles GET /api/auth/config: what the login page may offer.
func (h *Handler) Config(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"passkeys": h.svc.PasskeysEnabled()})
}

// RequestLink handles POST /api/auth/request with body {"email": "..."}. It
// answers 202 for any well-formed address, revealing nothing about whether
// the account existed. It (re)sets the browser cookie the emailed link and
// code are bound to; a resend keeps the same value, so every email of one
// browser stays valid in it.
func (h *Handler) RequestLink(w http.ResponseWriter, r *http.Request) {
	var body RequestBody
	if err := httpx.DecodeJSON(w, r, &body, 4<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	nonce := h.browserNonce(r)
	if nonce == "" {
		var err error
		if nonce, err = secret(); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "could not send the sign-in link")
			return
		}
	}
	switch err := h.svc.RequestLink(r.Context(), body.Email, nonce); {
	case errors.Is(err, ErrInvalidEmail):
		httpx.Error(w, http.StatusBadRequest, "invalid email address")
	case err != nil:
		h.log.Error("auth request link", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not send the sign-in link")
	default:
		http.SetCookie(w, h.browserCookie(nonce))
		httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "sent"})
	}
}

// Callback handles GET /api/auth/callback?token=... - the link shape older
// emails still carry. It never signs anyone in: a GET is what mail scanners
// prefetch, so it only forwards to the confirmation page.
func (h *Handler) Callback(w http.ResponseWriter, r *http.Request) {
	tok := r.URL.Query().Get("token")
	if !wellFormedToken(tok) {
		http.Redirect(w, r, h.svc.FrontendURL()+"/login?error=invalid_link", http.StatusSeeOther)
		return
	}
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, h.svc.ConfirmURL(tok), http.StatusSeeOther)
}

type tokenBody struct {
	Token string `json:"token"`
}

// Verify handles POST /api/auth/verify {"token": "..."}: which account a
// still-valid sign-in link belongs to, and whether this is the browser that
// asked for it - without spending it.
func (h *Handler) Verify(w http.ResponseWriter, r *http.Request) {
	var body tokenBody
	if err := httpx.DecodeJSON(w, r, &body, 4<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email, same, err := h.svc.Peek(r.Context(), body.Token, h.browserNonce(r))
	switch {
	case errors.Is(err, ErrInvalidToken):
		httpx.Error(w, http.StatusBadRequest, "invalid or expired link")
	case err != nil:
		h.log.Error("auth verify", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not check the link")
	default:
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"email": email, "sameBrowser": same})
	}
}

// Complete handles POST /api/auth/complete {"token": "..."}: spend the link
// and sign in - from the browser that requested it only (409
// "other_browser" otherwise, the link stays usable).
func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	var body tokenBody
	if err := httpx.DecodeJSON(w, r, &body, 4<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	res, err := h.svc.Complete(r.Context(), body.Token, h.browserNonce(r), h.client(r))
	switch {
	case errors.Is(err, ErrOtherBrowser):
		httpx.Error(w, http.StatusConflict, "other_browser")
	case errors.Is(err, ErrInvalidToken):
		httpx.Error(w, http.StatusBadRequest, "invalid or expired link")
	case err != nil:
		h.log.Error("auth complete", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not sign in")
	default:
		h.writeSignIn(w, r, res)
	}
}

type codeBody struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

// CompleteCode handles POST /api/auth/code {"email", "code"}: sign in with
// the 6-digit code from the email, in the browser that asked for it.
func (h *Handler) CompleteCode(w http.ResponseWriter, r *http.Request) {
	var body codeBody
	if err := httpx.DecodeJSON(w, r, &body, 4<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	res, err := h.svc.CompleteWithCode(r.Context(), body.Email, body.Code, h.browserNonce(r), h.client(r))
	switch {
	case errors.Is(err, ErrInvalidCode):
		httpx.Error(w, http.StatusBadRequest, "invalid or expired code")
	case err != nil:
		h.log.Error("auth code", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not sign in")
	default:
		h.writeSignIn(w, r, res)
	}
}

// writeSignIn answers a finished email step: either the session cookie, or
// the passkey challenge the account requires next.
func (h *Handler) writeSignIn(w http.ResponseWriter, r *http.Request, res SignIn) {
	http.SetCookie(w, h.clearBrowserCookie())
	if res.Passkey != nil {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "passkey_required", "passkey": res.Passkey})
		return
	}
	h.writeSession(w, r, res.SessionSecret, res.ExpiresAt)
}

// writeSession hands the new session over: a cookie for the browser, and the
// same secret as a bearer token when the installed app asked for it.
func (h *Handler) writeSession(w http.ResponseWriter, r *http.Request, secret string, exp time.Time) {
	http.SetCookie(w, h.sessionCookie(secret, exp))
	if isApp(r) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "signed_in", "token": secret, "expiresAt": exp.UTC()})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "signed_in"})
}

// BearerToken reads "Authorization: Bearer <secret>" - the installed app's
// session; "" when absent.
func BearerToken(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if len(v) > 7 && strings.EqualFold(v[:7], "bearer ") {
		return strings.TrimSpace(v[7:])
	}
	return ""
}

// PasskeyLoginBegin handles POST /api/auth/passkey/begin: options for a
// passkey-only sign-in.
func (h *Handler) PasskeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	ch, err := h.svc.BeginPasskeyLogin(r.Context())
	if err != nil {
		h.passkeyError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ch)
}

type ceremonyBody struct {
	CeremonyID string          `json:"ceremonyId"`
	Credential json.RawMessage `json:"credential"`
	Name       string          `json:"name"`
}

// PasskeyLoginFinish handles POST /api/auth/passkey/finish {ceremonyId,
// credential}: either kind of passkey sign-in (alone, or the second step).
func (h *Handler) PasskeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	var body ceremonyBody
	if err := httpx.DecodeJSON(w, r, &body, 64<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	secret, exp, err := h.svc.FinishPasskeyLogin(r.Context(), body.CeremonyID, body.Credential, h.client(r))
	if err != nil {
		h.passkeyError(w, err)
		return
	}
	h.writeSession(w, r, secret, exp)
}

// RevokeFromAlert handles POST /api/auth/revoke {"token"}: the alert email's
// "this wasn't me" - ends every session of that account.
func (h *Handler) RevokeFromAlert(w http.ResponseWriter, r *http.Request) {
	var body tokenBody
	if err := httpx.DecodeJSON(w, r, &body, 4<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email, err := h.svc.RevokeFromAlert(r.Context(), body.Token)
	switch {
	case errors.Is(err, ErrInvalidToken):
		httpx.Error(w, http.StatusBadRequest, "invalid or expired link")
	case err != nil:
		h.log.Error("auth revoke from alert", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not sign out")
	default:
		http.SetCookie(w, h.clearCookie())
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"email": email})
	}
}

// Logout handles POST /api/auth/logout: drop the session, clear the cookie.
// Always 204.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	secret := BearerToken(r)
	if c, err := r.Cookie(h.cfg.SessionCookieName); err == nil && secret == "" {
		secret = c.Value
	}
	if secret != "" {
		if err := h.svc.Logout(r.Context(), secret); err != nil {
			h.log.Error("auth logout", "error", err)
		}
	}
	http.SetCookie(w, h.clearCookie())
	w.WriteHeader(http.StatusNoContent)
}

// ClearSession expires the session cookie on w (used by sign-out-everywhere,
// which the router owns because it needs the request's user).
func (h *Handler) ClearSession(w http.ResponseWriter) { http.SetCookie(w, h.clearCookie()) }

// --- account security (signed in) -----------------------------------------

// Sessions handles GET /api/account/sessions.
func (h *Handler) Sessions(w http.ResponseWriter, r *http.Request) {
	c, ok := callerFrom(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "not signed in")
		return
	}
	list, err := h.svc.ListSessions(r.Context(), c.userID, c.sessionID)
	if err != nil {
		h.log.Error("list sessions", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not list sessions")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, list)
}

// RevokeSession handles DELETE /api/account/sessions/{id}. Revoking the
// current session also clears its cookie.
func (h *Handler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	c, ok := callerFrom(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "not signed in")
		return
	}
	id := r.PathValue("id")
	switch err := h.svc.RevokeSession(r.Context(), c.userID, id); {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "session not found")
	case err != nil:
		h.log.Error("revoke session", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not end the session")
	default:
		if id == c.sessionID {
			http.SetCookie(w, h.clearCookie())
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// Activity handles GET /api/account/activity.
func (h *Handler) Activity(w http.ResponseWriter, r *http.Request) {
	c, ok := callerFrom(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "not signed in")
		return
	}
	list, err := h.svc.Activity(r.Context(), c.userID)
	if err != nil {
		h.log.Error("list activity", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not list activity")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, list)
}

// Passkeys handles GET /api/account/passkeys.
func (h *Handler) Passkeys(w http.ResponseWriter, r *http.Request) {
	c, ok := callerFrom(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "not signed in")
		return
	}
	list, err := h.svc.ListPasskeys(r.Context(), c.userID)
	if err != nil {
		h.log.Error("list passkeys", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not list passkeys")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, list)
}

// PasskeyRegisterBegin handles POST /api/account/passkeys/begin.
func (h *Handler) PasskeyRegisterBegin(w http.ResponseWriter, r *http.Request) {
	c, ok := callerFrom(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "not signed in")
		return
	}
	ch, err := h.svc.BeginPasskeyRegistration(r.Context(), c.userID)
	if err != nil {
		h.passkeyError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ch)
}

// PasskeyRegisterFinish handles POST /api/account/passkeys/finish
// {ceremonyId, credential, name}.
func (h *Handler) PasskeyRegisterFinish(w http.ResponseWriter, r *http.Request) {
	c, ok := callerFrom(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "not signed in")
		return
	}
	var body ceremonyBody
	if err := httpx.DecodeJSON(w, r, &body, 64<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	pk, err := h.svc.FinishPasskeyRegistration(r.Context(), c.userID, body.CeremonyID, body.Name, body.Credential)
	if err != nil {
		h.passkeyError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, pk)
}

// DeletePasskey handles DELETE /api/account/passkeys/{id}.
func (h *Handler) DeletePasskey(w http.ResponseWriter, r *http.Request) {
	c, ok := callerFrom(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "not signed in")
		return
	}
	switch err := h.svc.DeletePasskey(r.Context(), c.userID, r.PathValue("id")); {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "passkey not found")
	case err != nil:
		h.passkeyError(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// RequirePasskey handles PUT /api/account/require-passkey {"enabled": bool}.
func (h *Handler) RequirePasskey(w http.ResponseWriter, r *http.Request) {
	c, ok := callerFrom(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "not signed in")
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 1<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	u, err := h.svc.SetRequirePasskey(r.Context(), c.userID, body.Enabled)
	switch {
	case errors.Is(err, ErrNoPasskey):
		httpx.Error(w, http.StatusConflict, "add a passkey first")
	case err != nil:
		h.passkeyError(w, err)
	default:
		httpx.WriteJSON(w, http.StatusOK, u)
	}
}

func (h *Handler) passkeyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrPasskeysDisabled):
		httpx.Error(w, http.StatusNotImplemented, "passkeys are not available")
	case errors.Is(err, ErrInvalidPasskey):
		httpx.Error(w, http.StatusBadRequest, "passkey verification failed")
	default:
		h.log.Error("passkey", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "passkey operation failed")
	}
}

// --- cookies ------------------------------------------------------------------

// browserNonce reads this browser's sign-in cookie, if well-formed.
func (h *Handler) browserNonce(r *http.Request) string {
	if c, err := r.Cookie(browserCookie); err == nil && wellFormedToken(c.Value) {
		return c.Value
	}
	// The installed app keeps its own binding value and sends it instead.
	if v := r.Header.Get(appLoginHeader); wellFormedToken(v) {
		return v
	}
	return ""
}

func (h *Handler) browserCookie(value string) *http.Cookie {
	ttl := h.cfg.MagicLinkTTL.Std()
	return &http.Cookie{
		Name:     browserCookie,
		Value:    value,
		Path:     "/api/auth",
		MaxAge:   int(ttl.Seconds()),
		Expires:  time.Now().Add(ttl),
		HttpOnly: true,
		Secure:   h.cfg.SessionCookieSecure,
		SameSite: http.SameSiteStrictMode,
	}
}

func (h *Handler) clearBrowserCookie() *http.Cookie {
	return &http.Cookie{
		Name: browserCookie, Value: "", Path: "/api/auth", MaxAge: -1,
		HttpOnly: true, Secure: h.cfg.SessionCookieSecure, SameSite: http.SameSiteStrictMode,
	}
}

// wellFormedToken accepts only what secret() produces (URL-safe base64), so
// nothing else is ever reflected into a redirect.
func wellFormedToken(t string) bool {
	if len(t) < 16 || len(t) > 128 {
		return false
	}
	for _, c := range t {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func (h *Handler) sessionCookie(value string, exp time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     h.cfg.SessionCookieName,
		Value:    value,
		Path:     "/",
		Expires:  exp,
		MaxAge:   int(time.Until(exp).Seconds()),
		HttpOnly: true,
		Secure:   h.cfg.SessionCookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
}

func (h *Handler) clearCookie() *http.Cookie {
	return &http.Cookie{
		Name:     h.cfg.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cfg.SessionCookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
}
