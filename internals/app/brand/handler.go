package brand

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"sade/internals/httpx"
)

type ctxKey struct{}

// WithUserID attaches the signed-in account's id (the router's job, so this
// package never imports the auth layer - same pattern as job.WithUserID).
func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

func userID(r *http.Request) (string, bool) {
	id, ok := r.Context().Value(ctxKey{}).(string)
	return id, ok && id != ""
}

// Handler is the account's brand settings API. Routes in
// internals/app/router.go, all behind a session.
type Handler struct {
	svc *Service
	log *slog.Logger
}

func NewHandler(svc *Service, log *slog.Logger) *Handler { return &Handler{svc: svc, log: log} }

func (h *Handler) caller(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, ok := userID(r)
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "not signed in")
	}
	return id, ok
}

func (h *Handler) reply(w http.ResponseWriter, resp Response, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		httpx.Error(w, http.StatusBadRequest, "invalid brand settings")
	case errors.Is(err, ErrInvalidLogo):
		httpx.Error(w, http.StatusUnsupportedMediaType, "logo must be a PNG or JPEG image")
	case errors.Is(err, ErrLogoTooLarge):
		httpx.Error(w, http.StatusRequestEntityTooLarge, "logo file is too large")
	case err != nil:
		h.log.Error("brand", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not save the brand")
	default:
		httpx.WriteJSON(w, http.StatusOK, resp)
	}
}

// Get handles GET /api/brand.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if uid, ok := h.caller(w, r); ok {
		resp, err := h.svc.Get(uid)
		h.reply(w, resp, err)
	}
}

// Update handles PUT /api/brand.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.caller(w, r)
	if !ok {
		return
	}
	var req UpdateRequest
	if err := httpx.DecodeJSON(w, r, &req, 4<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	resp, err := h.svc.Update(uid, req)
	h.reply(w, resp, err)
}

// UploadLogo handles POST /api/brand/logo (multipart, field "logo").
func (h *Handler) UploadLogo(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.caller(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxLogoBytes+64<<10)
	f, _, err := r.FormFile("logo")
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			h.reply(w, Response{}, ErrLogoTooLarge)
			return
		}
		httpx.Error(w, http.StatusBadRequest, "missing logo file")
		return
	}
	defer f.Close()
	resp, err := h.svc.SetLogo(r.Context(), uid, f)
	h.reply(w, resp, err)
}

// DeleteLogo handles DELETE /api/brand/logo.
func (h *Handler) DeleteLogo(w http.ResponseWriter, r *http.Request) {
	if uid, ok := h.caller(w, r); ok {
		resp, err := h.svc.RemoveLogo(uid)
		h.reply(w, resp, err)
	}
}

// Logo handles GET /api/brand/logo: the account's own logo, for the
// settings preview.
func (h *Handler) Logo(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.caller(w, r)
	if !ok {
		return
	}
	rc, err := h.svc.Logo(r.Context(), uid)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "no logo")
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable") // the URL carries ?v=<hash>
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if _, err := io.Copy(w, rc); err != nil {
		h.log.Warn("brand: stream logo", "error", err)
	}
}

// Onboarded handles POST /api/brand/onboarded: the welcome setup is done.
func (h *Handler) Onboarded(w http.ResponseWriter, r *http.Request) {
	if uid, ok := h.caller(w, r); ok {
		resp, err := h.svc.CompleteOnboarding(uid)
		h.reply(w, resp, err)
	}
}
