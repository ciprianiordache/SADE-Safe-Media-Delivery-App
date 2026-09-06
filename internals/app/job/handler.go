package job

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"sade/config"
	"sade/internals/httpx"
)

// Handler is the operator-facing HTTP surface for jobs: upload a file (which
// creates the job and its original asset), list your jobs, inspect one. All
// three routes are owner-scoped; the router attaches the operator id with
// WithUserID before dispatching. Routes are registered in
// internals/app/router.go.
type Handler struct {
	svc Service
	cfg config.UploadConfig
	log *slog.Logger
}

func NewHandler(svc Service, cfg config.UploadConfig, log *slog.Logger) *Handler {
	return &Handler{svc: svc, cfg: cfg, log: log}
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

func userIDFrom(r *http.Request) (string, bool) {
	id, ok := r.Context().Value(userIDKey).(string)
	return id, ok && id != ""
}

// --- handlers ----------------------------------------------------------

// Create handles POST /api/jobs. The body is a multipart form with a "file"
// part plus recipientEmail / watermarkKind / watermarkText / watermarkOpts
// fields. On success it returns 201 with the job and its original asset.
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
		WatermarkOpts:  r.FormValue("watermarkOpts"),
		Filename:       header.Filename,
		DeclaredSize:   header.Size,
		File:           file,
	})
	switch {
	case errors.Is(err, ErrInvalidInput):
		httpx.Error(w, http.StatusBadRequest, "invalid recipient email or watermark kind")
	case errors.Is(err, ErrUnsupportedMedia):
		httpx.Error(w, http.StatusUnsupportedMediaType, "unsupported file type")
	case errors.Is(err, ErrFileTooLarge):
		httpx.Error(w, http.StatusRequestEntityTooLarge, "file exceeds the size limit")
	case errors.Is(err, ErrEmptyFile):
		httpx.Error(w, http.StatusBadRequest, "the uploaded file is empty")
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
