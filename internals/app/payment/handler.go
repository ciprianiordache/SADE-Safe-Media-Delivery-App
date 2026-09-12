package payment

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	"sade/internals/httpx"
)

// maxWebhookBody bounds a Stripe webhook payload. Stripe's own events are a
// few KB; this is generous headroom, not a real limit.
const maxWebhookBody = 1 << 20

// Handler is the public, login-free HTTP surface for payments: a recipient
// on /preview/[token] never has a session, so every route here is
// authorised by the same signed preview token that page already holds (or,
// for the webhook, by Stripe's own request signature). Routes are
// registered in internals/app/router.go.
type Handler struct {
	svc Service
	log *slog.Logger
}

func NewHandler(svc Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

type checkoutRequest struct {
	Token string `json:"token"`
}

type checkoutResponse struct {
	CheckoutURL string `json:"checkoutUrl"`
}

// Checkout handles POST /api/payments/checkout with body {"token": "..."} -
// the preview token from the /preview/[token] URL the recipient is on.
func (h *Handler) Checkout(w http.ResponseWriter, r *http.Request) {
	var body checkoutRequest
	if err := httpx.DecodeJSON(w, r, &body, 4<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	url, err := h.svc.Checkout(r.Context(), body.Token)
	switch {
	case errors.Is(err, ErrDisabled):
		httpx.Error(w, http.StatusServiceUnavailable, "payment is not configured")
	case errors.Is(err, ErrInvalidToken):
		httpx.Error(w, http.StatusNotFound, "not found")
	case err != nil:
		h.log.Error("payment checkout", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not start checkout")
	default:
		httpx.WriteJSON(w, http.StatusOK, checkoutResponse{CheckoutURL: url})
	}
}

type statusResponse struct {
	Enabled     bool   `json:"enabled"`
	Paid        bool   `json:"paid"`
	OriginalURL string `json:"originalUrl,omitempty"`
	AmountCents int64  `json:"amountCents"`
	Currency    string `json:"currency"`
}

// Status handles GET /api/payments/status?token=... - the /preview/[token]
// page calls this on load (and after returning from Stripe Checkout) to
// decide between showing the "Unlock" CTA and the unlocked download state.
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	result, err := h.svc.Status(r.Context(), r.URL.Query().Get("token"))
	switch {
	case errors.Is(err, ErrInvalidToken):
		httpx.Error(w, http.StatusNotFound, "not found")
	case err != nil:
		h.log.Error("payment status", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "could not load payment status")
	default:
		httpx.WriteJSON(w, http.StatusOK, statusResponse{
			Enabled: result.Enabled, Paid: result.Paid, OriginalURL: result.OriginalURL,
			AmountCents: result.AmountCents, Currency: result.Currency,
		})
	}
}

// Webhook handles POST /api/payments/webhook - Stripe's own server calling
// back, authenticated by the Stripe-Signature header, never a session or
// token. Always reads the whole body first (signature verification needs
// the exact bytes) before handing it to the service.
func (h *Handler) Webhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBody+1))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "could not read request body")
		return
	}
	if len(payload) > maxWebhookBody {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "payload too large")
		return
	}
	err = h.svc.Webhook(r.Context(), payload, r.Header.Get("Stripe-Signature"))
	switch {
	case errors.Is(err, ErrDisabled):
		httpx.Error(w, http.StatusNotImplemented, "webhook not configured")
	case err != nil:
		h.log.Warn("payment webhook rejected", "error", err)
		httpx.Error(w, http.StatusBadRequest, "invalid webhook")
	default:
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
