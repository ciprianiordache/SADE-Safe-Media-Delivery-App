package share

import (
	"errors"
	"io"
	"net/http"
	"net/url"

	"sade/internals/app/asset"
	"sade/internals/app/job"
	"sade/internals/httpx"
	"sade/internals/wmopts"
)

// JobReader loads a job row. job.Repo satisfies it.
type JobReader interface {
	GetByID(id string) (*job.Job, error)
}

// UseBranding lets the recipient's page show who sent the file: the brand
// frozen on the job at upload time (internals/wmopts).
func (h *Handler) UseBranding(jobs JobReader) { h.jobs = jobs }

// PublicBrand is GET /api/public/brand?token=<preview token>.
type PublicBrand struct {
	Company string `json:"company,omitempty"`
	LogoURL string `json:"logoUrl,omitempty"`
}

func (h *Handler) brandOf(r *http.Request) (wmopts.Opts, error) {
	if h.jobs == nil {
		return wmopts.Opts{}, nil
	}
	a, err := h.resolve(r.URL.Query().Get("token"), "preview", asset.KindPreview)
	if err != nil {
		return wmopts.Opts{}, err
	}
	j, err := h.jobs.GetByID(a.JobID)
	if err != nil {
		return wmopts.Opts{}, errNotFound
	}
	return wmopts.Decode(j.WatermarkOpts), nil
}

func (h *Handler) brandError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errGone), errors.Is(err, errDisabled):
		httpx.Error(w, http.StatusGone, "this link is no longer active")
	case errors.Is(err, errNotFound):
		httpx.Error(w, http.StatusNotFound, "not found")
	default:
		h.log.Error("share: brand", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not load the brand")
	}
}

// Brand handles GET /api/public/brand?token=...: the sender's company name
// and a logo URL for the recipient's page.
func (h *Handler) Brand(w http.ResponseWriter, r *http.Request) {
	b, err := h.brandOf(r)
	if err != nil {
		h.brandError(w, err)
		return
	}
	out := PublicBrand{Company: b.Company}
	if b.LogoKey != "" {
		out.LogoURL = "/api/public/brand/logo?token=" + url.QueryEscape(r.URL.Query().Get("token"))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// BrandLogo handles GET /api/public/brand/logo?token=...
func (h *Handler) BrandLogo(w http.ResponseWriter, r *http.Request) {
	b, err := h.brandOf(r)
	if err != nil {
		h.brandError(w, err)
		return
	}
	if b.LogoKey == "" {
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	}
	rc, err := h.blob.Open(r.Context(), b.LogoKey)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, rc)
}
