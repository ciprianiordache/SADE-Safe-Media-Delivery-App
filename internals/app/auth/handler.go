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

// Callback handles GET /api/auth/callback?token=... On success it sets the
// session cookie and redirects to the frontend; a bad token redirects to the
// frontend login page with an error marker.
func (h *Handler) Callback(w http.ResponseWriter, r *http.Request) {
	secret, exp, err := h.svc.Complete(r.Context(), r.URL.Query().Get("token"))
	if err != nil {
		if !errors.Is(err, ErrInvalidToken) {
			h.log.Error("auth callback", "error", err)
		}
		http.Redirect(w, r, h.svc.FrontendURL()+"/login?error=invalid_link", http.StatusSeeOther)
		return
	}
	http.SetCookie(w, h.sessionCookie(secret, exp))
	http.Redirect(w, r, h.svc.FrontendURL(), http.StatusSeeOther)
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
