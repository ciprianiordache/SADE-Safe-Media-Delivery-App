package auth

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"sade/config"
	"sade/internals/httpx"
)

// Handler is the HTTP surface of the login flow: request a link, complete the
// callback, log out. The current-user endpoint (/api/me) lives in the router
// layer, since it only reads the user the Auth middleware already attached.
// Routes are registered in internals/app/router.go.
type Handler struct {
	svc *Service
	cfg config.AuthConfig
	log *slog.Logger
}

func NewHandler(svc *Service, cfg config.AuthConfig, log *slog.Logger) *Handler {
	return &Handler{svc: svc, cfg: cfg, log: log}
}

// RequestLink handles POST /api/auth/request with body {"email": "..."}. It
// answers 202 for any well-formed address, revealing nothing about whether
// the account existed.
func (h *Handler) RequestLink(w http.ResponseWriter, r *http.Request) {
	var body RequestBody
	if err := httpx.DecodeJSON(w, r, &body, 4<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	switch err := h.svc.RequestLink(r.Context(), body.Email); {
	case errors.Is(err, ErrInvalidEmail):
		httpx.Error(w, http.StatusBadRequest, "invalid email address")
	case err != nil:
		h.log.Error("auth request link", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not send the sign-in link")
	default:
		httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "sent"})
	}
}

// Callback handles GET /api/auth/callback?token=... - the link shape older
// emails still carry. It no longer signs anyone in: a GET is what mail
// scanners prefetch and what a third party can plant in an <img>, so it only
// forwards the browser to the confirmation page (token in the fragment),
// where the user sees the account and confirms with a click.
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
// still-valid sign-in link belongs to, without spending it.
func (h *Handler) Verify(w http.ResponseWriter, r *http.Request) {
	var body tokenBody
	if err := httpx.DecodeJSON(w, r, &body, 4<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email, err := h.svc.Peek(r.Context(), body.Token)
	switch {
	case errors.Is(err, ErrInvalidToken):
		httpx.Error(w, http.StatusBadRequest, "invalid or expired link")
	case err != nil:
		h.log.Error("auth verify", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not check the link")
	default:
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"email": email})
	}
}

// Complete handles POST /api/auth/complete {"token": "..."}: spend the
// sign-in token and set the session cookie. Called by the confirmation page
// on the user's click.
func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	var body tokenBody
	if err := httpx.DecodeJSON(w, r, &body, 4<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	secret, exp, err := h.svc.Complete(r.Context(), body.Token)
	switch {
	case errors.Is(err, ErrInvalidToken):
		httpx.Error(w, http.StatusBadRequest, "invalid or expired link")
	case err != nil:
		h.log.Error("auth complete", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not sign in")
	default:
		http.SetCookie(w, h.sessionCookie(secret, exp))
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "signed_in"})
	}
}

// ClearSession expires the session cookie on w (used by sign-out-everywhere,
// which the router owns because it needs the request's user).
func (h *Handler) ClearSession(w http.ResponseWriter) { http.SetCookie(w, h.clearCookie()) }

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

// Logout handles POST /api/auth/logout: drop the session, clear the cookie.
// Always 204.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(h.cfg.SessionCookieName); err == nil {
		if err := h.svc.Logout(r.Context(), c.Value); err != nil {
			h.log.Error("auth logout", "error", err)
		}
	}
	http.SetCookie(w, h.clearCookie())
	w.WriteHeader(http.StatusNoContent)
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
