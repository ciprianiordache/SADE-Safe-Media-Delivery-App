// Package payment unlocks a job's original file behind a Stripe Checkout
// payment. It never handles card data itself - the client is redirected to
// Stripe's hosted Checkout page and back; SADE only ever sees a Checkout
// Session id and its payment_status.
package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"sade/config"
	"sade/internals/app/asset"
	"sade/internals/token"

	stripe "github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"
)

// Service is payment's use cases: start (or resume) a checkout for a job's
// original file, report whether it has been paid for, and process the
// provider's webhook.
type Service interface {
	// Checkout resolves previewToken (the same signed token the recipient's
	// /preview page already holds) to a job, records a pending Payment, and
	// returns the Stripe-hosted Checkout URL to redirect the browser to.
	// ErrDisabled if no Stripe secret key is configured; ErrInvalidToken if
	// previewToken doesn't verify.
	Checkout(ctx context.Context, previewToken string) (checkoutURL string, err error)
	// Status reports whether the job behind previewToken has been paid for
	// and, if so, mints a fresh signed link to the original file. Safe to
	// call on every page load: a still-pending payment is reconciled by
	// asking Stripe directly for its Checkout Session, so a deployment with
	// no public webhook URL still completes the unlock.
	Status(ctx context.Context, previewToken string) (StatusResult, error)
	// Webhook verifies and processes one Stripe event payload. ErrDisabled
	// if no webhook signing secret is configured - refusing to process an
	// event it cannot verify, rather than trusting an unsigned body.
	Webhook(ctx context.Context, payload []byte, sigHeader string) error
}

// StatusResult is what the client-facing /preview page needs to render
// either the "Unlock" CTA or the unlocked download state.
type StatusResult struct {
	Enabled     bool // false when Stripe isn't configured at all - hide the CTA
	Paid        bool
	OriginalURL string // set only when Paid
	AmountCents int64
	Currency    string
}

type service struct {
	repo        Repo
	assets      asset.Repo
	signer      *token.Signer
	stripe      *stripe.Client
	cfg         config.PaymentConfig
	publicURL   string
	frontendURL string
	shareTTL    time.Duration
	log         *slog.Logger
}

// NewService wires the payment service. stripeClient may be nil when
// cfg.StripeSecretKey is empty - enabled() gates every path that would use
// it. shareTokenTTL matches whatever internals/token TTL /p and /d already
// use (Auth.ShareTokenTTL) so an unlocked original link behaves the same way.
func NewService(
	repo Repo,
	assets asset.Repo,
	signer *token.Signer,
	stripeClient *stripe.Client,
	cfg config.PaymentConfig,
	publicURL, frontendURL string,
	shareTokenTTL time.Duration,
	log *slog.Logger,
) Service {
	return &service{
		repo: repo, assets: assets, signer: signer, stripe: stripeClient, cfg: cfg,
		publicURL: publicURL, frontendURL: frontendURL, shareTTL: shareTokenTTL, log: log,
	}
}

func (s *service) enabled() bool { return s.cfg.StripeSecretKey != "" }

// resolvePreview verifies previewToken and returns the preview asset it
// authorises plus its job's original asset - the pair every payment
// operation needs (the job id from one, the file to unlock from the other).
func (s *service) resolvePreview(previewToken string) (previewAsset, originalAsset *asset.Asset, err error) {
	subject, err := s.signer.Verify("preview", previewToken)
	if err != nil {
		return nil, nil, ErrInvalidToken
	}
	pa, err := s.assets.GetByID(subject)
	if err != nil || pa.Kind != asset.KindPreview {
		return nil, nil, ErrInvalidToken
	}
	oa, err := s.assets.GetByJobAndKind(pa.JobID, asset.KindOriginal)
	if err != nil {
		return nil, nil, fmt.Errorf("payment: load original asset: %w", err)
	}
	return pa, oa, nil
}

func (s *service) Checkout(ctx context.Context, previewToken string) (string, error) {
	if !s.enabled() {
		return "", ErrDisabled
	}
	previewAsset, _, err := s.resolvePreview(previewToken)
	if err != nil {
		return "", err
	}
	jobID := previewAsset.JobID

	p := &Payment{
		JobID: jobID, Provider: ProviderStripe, Status: StatusPending,
		AmountCents: s.cfg.PriceCents, Currency: s.cfg.Currency,
	}
	id, err := s.repo.Create(p)
	if err != nil {
		return "", fmt.Errorf("payment: create row: %w", err)
	}
	p.ID = id

	params := &stripe.CheckoutSessionCreateParams{
		Mode: stripe.String(string(stripe.CheckoutSessionModePayment)),
		LineItems: []*stripe.CheckoutSessionCreateLineItemParams{
			{
				Quantity: stripe.Int64(1),
				PriceData: &stripe.CheckoutSessionCreateLineItemPriceDataParams{
					Currency:   stripe.String(s.cfg.Currency),
					UnitAmount: stripe.Int64(s.cfg.PriceCents),
					ProductData: &stripe.CheckoutSessionCreateLineItemPriceDataProductDataParams{
						Name: stripe.String("SADE — original file"),
					},
				},
			},
		},
		SuccessURL: stripe.String(s.frontendURL + "/preview/" + previewToken + "?paid=1"),
		CancelURL:  stripe.String(s.frontendURL + "/preview/" + previewToken),
		Metadata:   map[string]string{"payment_id": id, "job_id": jobID},
	}
	session, err := s.stripe.V1CheckoutSessions.Create(ctx, params)
	if err != nil {
		s.log.Error("payment: create checkout session", "payment", id, "error", err)
		return "", fmt.Errorf("payment: create checkout session: %w", err)
	}

	p.ProviderRef = session.ID
	if err := s.repo.Update(p); err != nil {
		// Non-fatal: Status's Stripe-side reconciliation still works via
		// LatestByJob once we retry recording this, but log it loudly since
		// until it's set, ByProviderRef (the webhook's own lookup) can't
		// find this row either.
		s.log.Error("payment: record provider ref", "payment", id, "session", session.ID, "error", err)
	}
	s.log.Info("payment: checkout started", "payment", id, "job", jobID, "session", session.ID)
	return session.URL, nil
}

func (s *service) Status(ctx context.Context, previewToken string) (StatusResult, error) {
	if !s.enabled() {
		return StatusResult{}, nil
	}
	previewAsset, originalAsset, err := s.resolvePreview(previewToken)
	if err != nil {
		return StatusResult{}, err
	}

	p, err := s.repo.LatestByJob(previewAsset.JobID)
	if errors.Is(err, ErrNotFound) {
		return StatusResult{Enabled: true, AmountCents: s.cfg.PriceCents, Currency: s.cfg.Currency}, nil
	}
	if err != nil {
		return StatusResult{}, fmt.Errorf("payment: load latest: %w", err)
	}

	if p.Status == StatusPending && p.ProviderRef != "" {
		s.reconcile(ctx, p)
	}

	if p.Status != StatusPaid {
		return StatusResult{Enabled: true, AmountCents: p.AmountCents, Currency: p.Currency}, nil
	}

	tok, err := s.signer.Sign("original", originalAsset.ID, s.shareTTL)
	if err != nil {
		return StatusResult{}, fmt.Errorf("payment: sign original token: %w", err)
	}
	return StatusResult{
		Enabled: true, Paid: true, OriginalURL: s.publicURL + "/o/" + tok,
		AmountCents: p.AmountCents, Currency: p.Currency,
	}, nil
}

// reconcile asks Stripe directly whether a still-pending Checkout Session
// has been paid, and marks the row paid if so. This is what makes the unlock
// work end to end even when Stripe has no public URL to send its webhook to
// (a local dev box, most of the time) - the recipient's own page load does
// the confirming instead.
func (s *service) reconcile(ctx context.Context, p *Payment) {
	session, err := s.stripe.V1CheckoutSessions.Retrieve(ctx, p.ProviderRef, nil)
	if err != nil {
		s.log.Error("payment: reconcile session", "payment", p.ID, "error", err)
		return
	}
	if session.PaymentStatus != stripe.CheckoutSessionPaymentStatusPaid {
		return
	}
	p.Status = StatusPaid
	if err := s.repo.Update(p); err != nil {
		s.log.Error("payment: mark paid on reconcile", "payment", p.ID, "error", err)
		return
	}
	s.log.Info("payment: reconciled as paid", "payment", p.ID, "job", p.JobID)
}

func (s *service) Webhook(_ context.Context, payload []byte, sigHeader string) error {
	if s.cfg.StripeWebhookSecret == "" {
		return ErrDisabled
	}
	// IgnoreAPIVersionMismatch: the Stripe account's pinned API version
	// (dashboard-configured, independent of this app) need not match the
	// version stripe-go was generated against - we only read a handful of
	// stable fields off the checkout.session, so a mismatch there is not a
	// reason to refuse a legitimately signed event.
	event, err := webhook.ConstructEventWithOptions(payload, sigHeader, s.cfg.StripeWebhookSecret,
		webhook.ConstructEventOptions{IgnoreAPIVersionMismatch: true})
	if err != nil {
		return fmt.Errorf("payment: verify webhook: %w", err)
	}
	if event.Type != "checkout.session.completed" {
		return nil
	}
	var session stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
		return fmt.Errorf("payment: decode checkout session: %w", err)
	}
	if session.PaymentStatus != stripe.CheckoutSessionPaymentStatusPaid {
		return nil
	}
	p, err := s.repo.ByProviderRef(session.ID)
	if err != nil {
		return fmt.Errorf("payment: load by provider ref %s: %w", session.ID, err)
	}
	if p.Status == StatusPaid {
		return nil // already reconciled (e.g. by Status's fallback) - idempotent
	}
	p.Status = StatusPaid
	if err := s.repo.Update(p); err != nil {
		return fmt.Errorf("payment: mark paid: %w", err)
	}
	s.log.Info("payment: webhook marked paid", "payment", p.ID, "job", p.JobID)
	return nil
}
