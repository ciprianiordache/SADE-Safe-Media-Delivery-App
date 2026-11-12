package share

import (
	"errors"
	"net/http"

	"sade/internals/app/asset"
	"sade/internals/httpx"
)

// Info is what the recipient's page says about a delivery before anything
// plays: the file and who sent it. The sender is the operator's account
// email - verified by their sign-in and already named in the email the
// recipient got - not a self-declared company name, which stays off the page
// until senders are verified (TODOS.md).
type Info struct {
	FileName  string `json:"fileName,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
	Sender    string `json:"sender,omitempty"`
}

// UseDetails installs the lookup Info uses (preview asset id -> delivery
// facts). Without it Info still answers, with the link state only.
func (h *Handler) UseDetails(f func(previewID string) (Info, error)) {
	h.details = f
}

// Info answers GET /api/share/info?token=<preview token>. It tells the
// recipient's page which state the link is in, so the page never shows a
// player for a file that cannot load:
//
//	200 {fileName, mediaType, sizeBytes, sender} - the link works
//	410 "expired"   - the link's time ran out (ask the sender for a new one)
//	410 "disabled"  - the sender switched the links off
//	404             - not a link SADE issued, or the file is gone
//
// It records no "opened" event: only fetching the file itself counts.
func (h *Handler) Info(w http.ResponseWriter, r *http.Request) {
	a, err := h.resolve(r.URL.Query().Get("token"), "preview", asset.KindPreview)
	switch {
	case errors.Is(err, errGone):
		httpx.Error(w, http.StatusGone, "expired")
		return
	case errors.Is(err, errDisabled):
		httpx.Error(w, http.StatusGone, "disabled")
		return
	case errors.Is(err, errNotFound):
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	case err != nil:
		h.log.Error("share: resolve info token", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not load the link")
		return
	}
	var info Info
	if h.details != nil {
		if d, err := h.details(a.ID); err == nil {
			info = d
		} else {
			h.log.Warn("share: delivery details", "asset", a.ID, "error", err)
		}
	}
	httpx.WriteJSON(w, http.StatusOK, info)
}
