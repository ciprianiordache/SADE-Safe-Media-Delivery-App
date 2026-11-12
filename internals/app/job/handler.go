package job

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"sade/config"
	"sade/internals/httpx"
	"sade/internals/storage"
)

// Handler is the operator-facing HTTP surface for jobs: upload a file (which
// creates the job and its original asset), list your jobs, inspect one,
// stream one of its assets. All routes are owner-scoped; the router attaches
// the operator id with WithUserID before dispatching. Routes are registered
// in internals/app/router.go.
type Handler struct {
	svc  Service
	cfg  config.UploadConfig
	blob storage.Storage
	log  *slog.Logger
}

func NewHandler(svc Service, blob storage.Storage, cfg config.UploadConfig, log *slog.Logger) *Handler {
	return &Handler{svc: svc, blob: blob, cfg: cfg, log: log}
}

// --- request-scoped operator id -----------------------------------------

type ctxKey int

const userIDKey ctxKey = iota

// WithUserID returns a copy of ctx carrying the authenticated operator's id.
// The router derives it from the session middleware and calls this before
// handing the request to a job handler, so the job package never imports the
// app/auth layer.
func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

type quotaExemptKey struct{}

// WithQuotaExempt marks ctx as belonging to an account the upload quotas do
// not apply to (admins). Set by the router, like WithUserID.
func WithQuotaExempt(ctx context.Context) context.Context {
	return context.WithValue(ctx, quotaExemptKey{}, true)
}

func quotaExempt(ctx context.Context) bool {
	v, _ := ctx.Value(quotaExemptKey{}).(bool)
	return v
}

func userIDFrom(r *http.Request) (string, bool) { return UserIDFrom(r.Context()) }

// UserIDFrom reads the operator id WithUserID attached - for other packages'
// operator-scoped handlers (internals/app/share's delivery routes).
func UserIDFrom(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDKey).(string)
	return id, ok && id != ""
}

// --- handlers ----------------------------------------------------------

// Create handles POST /api/jobs. The body is a multipart form with a "file"
// part plus recipientEmail / watermarkKind / watermarkText fields. On success it returns 201 with the job and its original asset.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "not signed in")
		return
	}

	memBuf := int64(h.cfg.MemoryBufMiB) << 20
	if maxBytes := int64(h.cfg.MaxSizeMiB) << 20; maxBytes > 0 {
		// Room for the file, the in-memory field buffer, and multipart
		// boundary overhead.
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes+memBuf+(1<<20))
	}
	if err := r.ParseMultipartForm(memBuf); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "missing \"file\" part")
		return
	}
	defer file.Close()

	resp, err := h.svc.Create(r.Context(), uid, NewUpload{
		RecipientEmail: r.FormValue("recipientEmail"),
		WatermarkKind:  r.FormValue("watermarkKind"),
		WatermarkText:  r.FormValue("watermarkText"),
		Filename:       header.Filename,
		DeclaredSize:   header.Size,
		File:           file,
	})
	switch {
	case errors.Is(err, ErrInvalidInput):
		httpx.Error(w, http.StatusBadRequest, "invalid recipient email, watermark kind, text or options")
	case errors.Is(err, ErrDailyLimit):
		httpx.Error(w, http.StatusTooManyRequests, "daily job limit reached")
	case errors.Is(err, ErrStorageQuota):
		httpx.Error(w, http.StatusRequestEntityTooLarge, "storage quota exceeded")
	case errors.Is(err, ErrUnsupportedMedia):
		httpx.Error(w, http.StatusUnsupportedMediaType, "unsupported file type")
	case errors.Is(err, ErrFileTooLarge):
		httpx.Error(w, http.StatusRequestEntityTooLarge, "file exceeds the size limit")
	case errors.Is(err, ErrEmptyFile):
		httpx.Error(w, http.StatusBadRequest, "the uploaded file is empty")
	case errors.Is(err, ErrCorruptMedia):
		httpx.Error(w, http.StatusUnsupportedMediaType, "file content does not match its extension")
	case err != nil:
		h.log.Error("create job", "user", uid, "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not create the job")
	default:
		httpx.WriteJSON(w, http.StatusCreated, resp)
	}
}

// List handles GET /api/jobs?offset=&limit=, scoped to the caller.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "not signed in")
		return
	}
	offset, limit := httpx.Page(r, 50, 200)
	jobs, err := h.svc.List(uid, offset, limit)
	if err != nil {
		h.log.Error("list jobs", "user", uid, "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not list jobs")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, jobs)
}

// Usage handles GET /api/usage: the caller's consumption against their quotas.
func (h *Handler) Usage(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "not signed in")
		return
	}
	u, err := h.svc.Usage(r.Context(), uid)
	if err != nil {
		h.log.Error("job usage", "user", uid, "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not load usage")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, u)
}

// Delete handles DELETE /api/jobs/{id}: remove one of the caller's jobs and
// its files. 204 on success, 404 for someone else's job, 409 while it is
// processing or once its original has been paid for.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "not signed in")
		return
	}
	err := h.svc.Delete(r.Context(), uid, r.PathValue("id"))
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "job not found")
	case errors.Is(err, ErrBusy):
		httpx.Error(w, http.StatusConflict, "job is being processed")
	case errors.Is(err, ErrPaid):
		httpx.Error(w, http.StatusConflict, "job has a paid unlock")
	case err != nil:
		h.log.Error("delete job", "user", uid, "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not delete the job")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// Get handles GET /api/jobs/{id}, scoped to the caller. A job owned by
// another operator is reported as 404.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "not signed in")
		return
	}
	resp, err := h.svc.Get(uid, r.PathValue("id"))
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "job not found")
	case err != nil:
		h.log.Error("get job", "user", uid, "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not load the job")
	default:
		httpx.WriteJSON(w, http.StatusOK, resp)
	}
}

// Content handles GET /api/jobs/{id}/assets/{assetId}/content, scoped to the
// caller: it streams either the original upload or the watermarked preview
// so the operator can play/compare both from the dashboard, session-
// authenticated (unlike the public GET /p/{token}, which only ever serves a
// preview asset to whoever holds the emailed link). ?dl=1 asks for an
// attachment instead of an inline response.
// AssetLink handles GET /api/jobs/{id}/assets/{assetId}/link: a short-lived
// URL of the content route that needs no session, for players that cannot
// send one (the installed app's <img>/<video>). Owner-scoped like Content;
// sign mints the link for (operator, asset).
func (h *Handler) AssetLink(sign func(userID, assetID string) (string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, ok := userIDFrom(r)
		if !ok {
			httpx.Error(w, http.StatusUnauthorized, "not signed in")
			return
		}
		a, err := h.svc.Asset(uid, r.PathValue("id"), r.PathValue("assetId"))
		switch {
		case errors.Is(err, ErrNotFound):
			httpx.Error(w, http.StatusNotFound, "not found")
			return
		case err != nil:
			h.log.Error("job asset link", "user", uid, "error", err)
			httpx.Error(w, http.StatusInternalServerError, "could not load the asset")
			return
		}
		t, err := sign(uid, a.ID)
		if err != nil {
			h.log.Error("job asset link: sign", "error", err)
			httpx.Error(w, http.StatusInternalServerError, "could not load the asset")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{
			"url": fmt.Sprintf("/api/jobs/%s/assets/%s/content?t=%s", url.PathEscape(r.PathValue("id")), url.PathEscape(a.ID), url.QueryEscape(t)),
		})
	}
}

func (h *Handler) Content(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "not signed in")
		return
	}
	a, err := h.svc.Asset(uid, r.PathValue("id"), r.PathValue("assetId"))
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	case err != nil:
		h.log.Error("job asset content", "user", uid, "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not load the asset")
		return
	}

	disposition := "inline"
	if r.URL.Query().Get("dl") != "" {
		disposition = "attachment"
	}
	mimeType := a.MIME
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, sanitizeFilename(a.Filename)))
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// Preferred path: a seekable local file, so http.ServeContent can honour
	// Range requests (scrubbing a video) and conditional GETs - see
	// internals/app/share.Handler.serve, which this mirrors.
	if path, ok := h.blob.LocalPath(a.StorageKey); ok {
		f, err := os.Open(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				httpx.Error(w, http.StatusNotFound, "not found")
				return
			}
			h.log.Error("job asset content: open", "key", a.StorageKey, "error", err)
			httpx.Error(w, http.StatusInternalServerError, "could not load the asset")
			return
		}
		defer f.Close()
		fi, err := f.Stat()
		if err != nil {
			h.log.Error("job asset content: stat", "key", a.StorageKey, "error", err)
			httpx.Error(w, http.StatusInternalServerError, "could not load the asset")
			return
		}
		http.ServeContent(w, r, a.Filename, fi.ModTime(), f)
		return
	}

	// Fallback for a non-seekable backend (S3, not implemented yet): whole-
	// object copy, no Range support.
	rc, err := h.blob.Open(r.Context(), a.StorageKey)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "not found")
			return
		}
		h.log.Error("job asset content: open", "key", a.StorageKey, "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not load the asset")
		return
	}
	defer rc.Close()
	if fi, sErr := h.blob.Stat(r.Context(), a.StorageKey); sErr == nil && fi.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(fi.Size, 10))
	}
	if _, err := io.Copy(w, rc); err != nil {
		h.log.Error("job asset content: stream", "key", a.StorageKey, "error", err)
	}
}

// sanitizeFilename drops quotes, separators and control characters so the
// value is safe inside a quoted Content-Disposition filename.
func sanitizeFilename(name string) string {
	clean := make([]rune, 0, len(name))
	for _, r := range name {
		if r < 0x20 || r == '"' || r == '\\' || r == '/' {
			continue
		}
		clean = append(clean, r)
	}
	if len(clean) == 0 {
		return "file"
	}
	return string(clean)
}
