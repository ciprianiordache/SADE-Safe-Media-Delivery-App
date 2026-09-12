package share

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"sade/internals/app/asset"
	"sade/internals/httpx"
	"sade/internals/storage"
	"sade/internals/token"
)

// Assets is the read port share needs: fetch one asset by id. asset.Repo
// satisfies it directly.
type Assets interface {
	GetByID(id string) (*asset.Asset, error)
}

// Blob is the subset of internals/storage share uses to stream bytes.
// *storage.Storage backends satisfy it directly.
type Blob interface {
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Stat(ctx context.Context, key string) (storage.FileInfo, error)
	LocalPath(key string) (path string, ok bool)
}

// Handler serves the public share routes. Routes are registered in
// internals/app/router.go, outside the /api auth concerns.
type Handler struct {
	signer *token.Signer
	assets Assets
	blob   Blob
	log    *slog.Logger
}

func NewHandler(signer *token.Signer, assets Assets, blob Blob, log *slog.Logger) *Handler {
	return &Handler{signer: signer, assets: assets, blob: blob, log: log}
}

// Preview handles GET /p/{token}: the watermarked preview, shown inline.
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	h.serve(w, r, "preview", asset.KindPreview, "inline")
}

// Download handles GET /d/{token}: the watermarked preview, as an attachment.
func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	h.serve(w, r, "download", asset.KindPreview, "attachment")
}

// Original handles GET /o/{token}: the clean, un-watermarked original, as an
// attachment. The token is minted by internals/app/payment.Service.Status
// only once a Payment for the job is StatusPaid - unlike Preview/Download,
// there is no free path to this purpose.
func (h *Handler) Original(w http.ResponseWriter, r *http.Request) {
	h.serve(w, r, "original", asset.KindOriginal, "attachment")
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request, purpose, wantKind, disposition string) {
	a, err := h.resolve(r.PathValue("token"), purpose, wantKind)
	switch {
	case errors.Is(err, errGone):
		httpx.Error(w, http.StatusGone, "this link has expired")
		return
	case errors.Is(err, errNotFound):
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	case err != nil:
		h.log.Error("share: resolve token", "purpose", purpose, "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not serve the file")
		return
	}

	w.Header().Set("Content-Type", mimeOr(a.MIME))
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("%s; filename=%q", disposition, sanitizeFilename(a.Filename)))
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// Preferred path: a seekable local file, so http.ServeContent can honour
	// Range requests (scrubbing a video preview) and conditional GETs.
	if path, ok := h.blob.LocalPath(a.StorageKey); ok {
		f, err := os.Open(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				httpx.Error(w, http.StatusNotFound, "not found")
				return
			}
			h.log.Error("share: open preview", "key", a.StorageKey, "error", err)
			httpx.Error(w, http.StatusInternalServerError, "could not serve the file")
			return
		}
		defer f.Close()
		fi, err := f.Stat()
		if err != nil {
			h.log.Error("share: stat preview", "key", a.StorageKey, "error", err)
			httpx.Error(w, http.StatusInternalServerError, "could not serve the file")
			return
		}
		http.ServeContent(w, r, a.Filename, fi.ModTime(), f)
		return
	}

	// Fallback for a non-seekable backend (S3, not implemented yet):
	// whole-object copy, no Range support.
	rc, err := h.blob.Open(r.Context(), a.StorageKey)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "not found")
			return
		}
		h.log.Error("share: open preview", "key", a.StorageKey, "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not serve the file")
		return
	}
	defer rc.Close()
	if fi, sErr := h.blob.Stat(r.Context(), a.StorageKey); sErr == nil && fi.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(fi.Size, 10))
	}
	if _, err := io.Copy(w, rc); err != nil {
		h.log.Error("share: stream preview", "key", a.StorageKey, "error", err)
	}
}

// resolve verifies the token for purpose and returns the asset it
// authorises, provided it is of wantKind. Every failure maps to errNotFound
// or errGone so a caller cannot probe which token / asset exists.
func (h *Handler) resolve(tok, purpose, wantKind string) (*asset.Asset, error) {
	if strings.TrimSpace(tok) == "" {
		return nil, errNotFound
	}
	subject, err := h.signer.Verify(purpose, tok)
	if err != nil {
		if errors.Is(err, token.ErrExpired) {
			return nil, errGone
		}
		return nil, errNotFound // bad signature, malformed, or wrong purpose
	}
	a, err := h.assets.GetByID(subject)
	if err != nil {
		if errors.Is(err, asset.ErrNotFound) {
			return nil, errNotFound
		}
		return nil, fmt.Errorf("load asset %s: %w", subject, err)
	}
	if a.Kind != wantKind {
		return nil, errNotFound // e.g. a "preview" token pointed at an original row
	}
	return a, nil
}

func mimeOr(m string) string {
	if strings.TrimSpace(m) == "" {
		return "application/octet-stream"
	}
	return m
}

// sanitizeFilename drops quotes, separators and control characters so the
// value is safe inside a quoted Content-Disposition filename.
func sanitizeFilename(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == '"' || r == '\\' || r == '/' {
			return -1
		}
		return r
	}, name)
	if name == "" {
		return "preview"
	}
	return name
}
