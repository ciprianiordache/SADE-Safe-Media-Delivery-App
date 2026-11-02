package share

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"sade/internals/app/asset"
	"sade/internals/app/job"
	"sade/internals/app/job_event"
	"sade/internals/httpx"
	"sade/internals/token"
)

// maxResendsPerHour bounds "resend the email" per job - it mails a third
// party, so it must not become a way to spam them.
const maxResendsPerHour = 3

// Jobs is the owner-scoped job read the delivery routes need. job.Service
// satisfies it: a job that isn't the caller's reads as job.ErrNotFound.
type Jobs interface {
	Get(userID, jobID string) (job.Response, error)
}

// JobFlags flips a job's links switch and tells whether it was paid for.
// job.Repo satisfies it.
type JobFlags interface {
	SetLinksDisabled(jobID string, disabled bool) error
	HasPaidPayment(jobID string) (bool, error)
}

// History is a job's delivery timeline. job_event.Repo satisfies it.
type History interface {
	Record(jobID, kind, detail string) error
	ListByJob(jobID string) ([]job_event.JobEvent, error)
	CountSince(jobID, kind string, since time.Time) (int, error)
}

// Notifier emails a recipient their preview links. worker.EmailNotifier
// satisfies it.
type Notifier interface {
	PreviewReady(ctx context.Context, recipient, previewID string) error
}

// OperatorHandler is the operator's side of delivery: the link they can
// copy, resending the email, switching links off and on, and the history.
// Every route is owner-scoped through Jobs and expects job.WithUserID on the
// context (the router's operator wrapper).
type OperatorHandler struct {
	jobs      Jobs
	flags     JobFlags
	history   History
	notifier  Notifier
	signer    *token.Signer
	publicURL string
	ttl       time.Duration
	log       *slog.Logger
}

func NewOperatorHandler(jobs Jobs, flags JobFlags, history History, notifier Notifier,
	signer *token.Signer, publicURL string, ttl time.Duration, log *slog.Logger) *OperatorHandler {
	return &OperatorHandler{
		jobs: jobs, flags: flags, history: history, notifier: notifier, signer: signer,
		publicURL: strings.TrimRight(publicURL, "/"), ttl: ttl, log: log,
	}
}

// EventView is one entry of the delivery timeline.
type EventView struct {
	Kind      string    `json:"kind"`
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// Delivery is GET /api/jobs/{id}/delivery's body.
type Delivery struct {
	Enabled bool `json:"enabled"`
	Paid    bool `json:"paid"`
	// Ready: the preview exists, so there is a link to share or resend.
	Ready bool `json:"ready"`
	// PreviewURL is a fresh link to the recipient's preview page, the same
	// kind the email carries; empty until Ready, or while links are off.
	PreviewURL string      `json:"previewUrl,omitempty"`
	ExpiresAt  time.Time   `json:"expiresAt,omitempty"`
	Events     []EventView `json:"events"`
}

// load resolves the caller's job (404 for anyone else's) and its preview.
func (h *OperatorHandler) load(w http.ResponseWriter, r *http.Request) (job.Response, string, bool) {
	uid, ok := job.UserIDFrom(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "not signed in")
		return job.Response{}, "", false
	}
	j, err := h.jobs.Get(uid, r.PathValue("id"))
	if errors.Is(err, job.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "job not found")
		return job.Response{}, "", false
	}
	if err != nil {
		h.log.Error("delivery: load job", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not load the job")
		return job.Response{}, "", false
	}
	previewID := ""
	for _, a := range j.Assets {
		if a.Kind == asset.KindPreview {
			previewID = a.ID
		}
	}
	return j, previewID, true
}

// Delivery handles GET /api/jobs/{id}/delivery.
func (h *OperatorHandler) Delivery(w http.ResponseWriter, r *http.Request) {
	j, previewID, ok := h.load(w, r)
	if !ok {
		return
	}
	paid, err := h.flags.HasPaidPayment(j.ID)
	if err != nil {
		h.log.Error("delivery: paid check", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not load delivery")
		return
	}
	events, err := h.history.ListByJob(j.ID)
	if err != nil {
		h.log.Error("delivery: history", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not load delivery")
		return
	}
	out := Delivery{Enabled: !j.LinksDisabled, Paid: paid, Ready: previewID != "", Events: make([]EventView, 0, len(events))}
	for _, e := range events {
		out.Events = append(out.Events, EventView{Kind: e.Kind, Detail: e.Detail, CreatedAt: e.CreatedAt})
	}
	if out.Ready && out.Enabled {
		tok, err := h.signer.Sign("preview", previewID, h.ttl)
		if err != nil {
			h.log.Error("delivery: sign link", "error", err)
			httpx.Error(w, http.StatusInternalServerError, "could not load delivery")
			return
		}
		out.PreviewURL = h.publicURL + "/preview/" + tok
		out.ExpiresAt = time.Now().Add(h.ttl).UTC()
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// Resend handles POST /api/jobs/{id}/resend: email the recipient fresh links.
func (h *OperatorHandler) Resend(w http.ResponseWriter, r *http.Request) {
	j, previewID, ok := h.load(w, r)
	if !ok {
		return
	}
	switch {
	case previewID == "":
		httpx.Error(w, http.StatusConflict, "preview not ready")
		return
	case j.LinksDisabled:
		httpx.Error(w, http.StatusConflict, "links are disabled")
		return
	}
	n, err := h.history.CountSince(j.ID, job_event.KindResent, time.Now().UTC().Add(-time.Hour))
	if err != nil {
		h.log.Error("delivery: count resends", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not resend")
		return
	}
	if n >= maxResendsPerHour {
		httpx.Error(w, http.StatusTooManyRequests, "resend limit reached")
		return
	}
	if err := h.notifier.PreviewReady(r.Context(), j.RecipientEmail, previewID); err != nil {
		h.log.Error("delivery: resend email", "job", j.ID, "error", err)
		httpx.Error(w, http.StatusBadGateway, "could not send the email")
		return
	}
	if err := h.history.Record(j.ID, job_event.KindResent, j.RecipientEmail); err != nil {
		h.log.Warn("delivery: record resend", "job", j.ID, "error", err)
	}
	h.log.Info("delivery: email resent", "job", j.ID)
	w.WriteHeader(http.StatusNoContent)
}

// SetLinks handles PUT /api/jobs/{id}/links {"enabled": bool}. Switching off
// is refused once the recipient has paid - they keep what they bought.
func (h *OperatorHandler) SetLinks(w http.ResponseWriter, r *http.Request) {
	j, _, ok := h.load(w, r)
	if !ok {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 1<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Enabled == !j.LinksDisabled {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !body.Enabled {
		paid, err := h.flags.HasPaidPayment(j.ID)
		if err != nil {
			h.log.Error("delivery: paid check", "error", err)
			httpx.Error(w, http.StatusInternalServerError, "could not change the links")
			return
		}
		if paid {
			httpx.Error(w, http.StatusConflict, "paid")
			return
		}
	}
	if err := h.flags.SetLinksDisabled(j.ID, !body.Enabled); err != nil {
		h.log.Error("delivery: set links", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not change the links")
		return
	}
	kind := job_event.KindLinksDisabled
	if body.Enabled {
		kind = job_event.KindLinksEnabled
	}
	if err := h.history.Record(j.ID, kind, ""); err != nil {
		h.log.Warn("delivery: record links change", "job", j.ID, "error", err)
	}
	h.log.Info("delivery: links switched", "job", j.ID, "enabled", body.Enabled)
	w.WriteHeader(http.StatusNoContent)
}
