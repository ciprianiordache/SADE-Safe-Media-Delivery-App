// Package payment unlocks a job's original file behind a Stripe payment. It
// never handles card data itself: card entry happens client-side, in the
// browser, via Stripe's Payment Element against a PaymentIntent's client
// secret - this package only ever sees a PaymentIntent id and its status.
// Unlike Stripe's hosted Checkout, the Payment Element mounts inside SADE's
// own page, so the surrounding design (and, via its appearance API, the
// form itself) stays SADE's.
package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"sade/config"
	"sade/internals/app/asset"
	"sade/internals/token"

	stripe "github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"
)

// Service is payment's use cases: start (or resume) paying for a job's
// original file, report whether it has been paid for, and process the
// provider's webhook.
type Service interface {
	// CreateIntent resolves previewToken (the same signed token the
	// recipient's /preview page already holds) to a job, records a pending
	// Payment, and returns a Stripe PaymentIntent's client secret - what the
	// frontend needs to mount the Payment Element and confirm the payment
	// itself. ErrDisabled if no Stripe secret key is configured;
	// ErrInvalidToken if previewToken doesn't verify.
	CreateIntent(ctx context.Context, previewToken string) (clientSecret string, err error)
	// Status reports whether the job behind previewToken has been paid for
	// and, if so, mints a fresh signed link to the original file. Safe to
	// call on every page load: a still-pending payment is reconciled by
	// asking Stripe directly for its PaymentIntent, so a deployment with no
	// public webhook URL still completes the unlock.
	Status(ctx context.Context, previewToken string) (StatusResult, error)
	// Webhook verifies and processes one Stripe event payload. ErrDisabled
	// if no webhook signing secret is configured - refusing to process an
	// event it cannot verify, rather than trusting an unsigned body.
	Webhook(ctx context.Context, payload []byte, sigHeader string) error
}

// StatusResult is what the client-facing /preview page needs to render
// either the "Unlock" form or the unlocked download state.
type StatusResult struct {
	Enabled        bool // false when Stripe isn't configured at all - hide the CTA
	Paid           bool
	OriginalURL    string // set only when Paid
	AmountCents    int64
	Currency       string
	PublishableKey string // set whenever Enabled - the frontend needs it to mount Stripe.js
}

type service struct {
	repo      Repo
	assets    asset.Repo
	signer    *token.Signer
	stripe    *stripe.Client
	cfg       config.PaymentConfig
	publicURL string
	shareTTL  time.Duration
	log       *slog.Logger
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
	publicURL string,
	shareTokenTTL time.Duration,
	log *slog.Logger,
) Service {
	return &service{
		repo: repo, assets: assets, signer: signer, stripe: stripeClient, cfg: cfg,
		publicURL: publicURL, shareTTL: shareTokenTTL, log: log,
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

func (s *service) CreateIntent(ctx context.Context, previewToken string) (string, error) {
	if !s.enabled() {
		return "", ErrDisabled
	}
	previewAsset, _, err := s.resolvePreview(previewToken)
	if err != nil {
		return "", err
	}
	jobID := previewAsset.JobID

	// Anyone holding the preview link can call this, so it must never be a
	// way to undo or duplicate a purchase: a paid job refuses outright, and a
	// still-open PaymentIntent is handed back instead of minting another
	// (which would also grow the payments table and Stripe's intent list on
	// every page load or scripted call).
	payments, err := s.repo.ListByJob(jobID)
	if err != nil {
		return "", fmt.Errorf("payment: list payments: %w", err)
	}
	if paidPayment(payments) != nil {
		return "", ErrAlreadyPaid
	}
	if secret, ok := s.reusableIntent(ctx, payments); ok {
		return secret, nil
	}

	p := &Payment{
		JobID: jobID, Provider: ProviderStripe, Status: StatusPending,
		AmountCents: s.cfg.PriceCents, Currency: s.cfg.Currency,
	}
	id, err := s.repo.Create(p)
	if err != nil {
		return "", fmt.Errorf("payment: create row: %w", err)
	}
	p.ID = id

	intent, err := s.stripe.V1PaymentIntents.Create(ctx, &stripe.PaymentIntentCreateParams{
		Amount:   stripe.Int64(s.cfg.PriceCents),
		Currency: stripe.String(s.cfg.Currency),
		AutomaticPaymentMethods: &stripe.PaymentIntentCreateAutomaticPaymentMethodsParams{
			Enabled: stripe.Bool(true),
		},
		Description: stripe.String("SADE — original file"),
		Metadata:    map[string]string{"payment_id": id, "job_id": jobID},
	})
	if err != nil {
		s.log.Error("payment: create payment intent", "payment", id, "error", err)
		return "", fmt.Errorf("payment: create payment intent: %w", err)
	}

	p.ProviderRef = intent.ID
	if err := s.repo.Update(p); err != nil {
		// Non-fatal: Status's Stripe-side reconciliation still works via
		// LatestByJob once we retry recording this, but log it loudly since
		// until it's set, ByProviderRef (the webhook's own lookup) can't
		// find this row either.
		s.log.Error("payment: record provider ref", "payment", id, "intent", intent.ID, "error", err)
	}
	s.log.Info("payment: intent created", "payment", id, "job", jobID, "intent", intent.ID)
	return intent.ClientSecret, nil
}

func (s *service) Status(ctx context.Context, previewToken string) (StatusResult, error) {
	if !s.enabled() {
		return StatusResult{}, nil
	}
	previewAsset, originalAsset, err := s.resolvePreview(previewToken)
	if err != nil {
		return StatusResult{}, err
	}

	payments, err := s.repo.ListByJob(previewAsset.JobID)
	if err != nil {
		return StatusResult{}, fmt.Errorf("payment: list payments: %w", err)
	}
	if len(payments) == 0 {
		return StatusResult{
			Enabled: true, PublishableKey: s.cfg.StripePublishableKey,
			AmountCents: s.cfg.PriceCents, Currency: s.cfg.Currency,
		}, nil
	}

	// Paid is a property of the job, not of its newest attempt: a later
	// pending row (someone reopening checkout) must never hide a purchase.
	p := paidPayment(payments)
	if p == nil {
		p = &payments[0]
		if p.Status == StatusPending && p.ProviderRef != "" {
			s.reconcile(ctx, p)
		}
	}

	if p.Status != StatusPaid {
		return StatusResult{
			Enabled: true, PublishableKey: s.cfg.StripePublishableKey,
			AmountCents: p.AmountCents, Currency: p.Currency,
		}, nil
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

// coversPayment reports whether a succeeded intent actually collected what p
// asked for. SADE creates every intent itself, so a mismatch means something
// changed it outside this code path - never treat that as a purchase.
func coversPayment(intent *stripe.PaymentIntent, p *Payment) bool {
	received := intent.AmountReceived
	if received == 0 {
		received = intent.Amount
	}
	return received >= p.AmountCents && strings.EqualFold(string(intent.Currency), p.Currency)
}

// paidPayment returns the first paid payment in payments, or nil.
func paidPayment(payments []Payment) *Payment {
	for i := range payments {
		if payments[i].Status == StatusPaid {
			return &payments[i]
		}
	}
	return nil
}

// reusableIntent returns the client secret of the newest pending payment's
// PaymentIntent when Stripe still accepts a confirmation on it, so a reload
// of the preview page keeps using one intent instead of piling up new ones.
func (s *service) reusableIntent(ctx context.Context, payments []Payment) (string, bool) {
	if len(payments) == 0 {
		return "", false
	}
	p := payments[0]
	if p.Status != StatusPending || p.ProviderRef == "" {
		return "", false
	}
	intent, err := s.stripe.V1PaymentIntents.Retrieve(ctx, p.ProviderRef, nil)
	if err != nil {
		s.log.Warn("payment: retrieve intent for reuse", "payment", p.ID, "error", err)
		return "", false
	}
	switch intent.Status {
	case stripe.PaymentIntentStatusRequiresPaymentMethod,
		stripe.PaymentIntentStatusRequiresConfirmation,
		stripe.PaymentIntentStatusRequiresAction:
		if intent.ClientSecret != "" && intent.Amount == s.cfg.PriceCents &&
			string(intent.Currency) == s.cfg.Currency {
			return intent.ClientSecret, true
		}
	}
	return "", false
}

// reconcile asks Stripe directly whether a still-pending PaymentIntent has
// succeeded, and marks the row paid if so. This is what makes the unlock
// work end to end even when Stripe has no public URL to send its webhook to
// (a local dev box, most of the time) - the recipient's own page load does
// the confirming instead.
func (s *service) reconcile(ctx context.Context, p *Payment) {
	intent, err := s.stripe.V1PaymentIntents.Retrieve(ctx, p.ProviderRef, nil)
	if err != nil {
		s.log.Error("payment: reconcile intent", "payment", p.ID, "error", err)
		return
	}
	if intent.Status != stripe.PaymentIntentStatusSucceeded {
		return
	}
	if !coversPayment(intent, p) {
		s.log.Error("payment: intent succeeded for the wrong amount", "payment", p.ID,
			"intent", intent.ID, "amount", intent.AmountReceived, "currency", intent.Currency)
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
	// stable fields off the payment_intent, so a mismatch there is not a
	// reason to refuse a legitimately signed event.
	event, err := webhook.ConstructEventWithOptions(payload, sigHeader, s.cfg.StripeWebhookSecret,
		webhook.ConstructEventOptions{IgnoreAPIVersionMismatch: true})
	if err != nil {
		return fmt.Errorf("payment: verify webhook: %w", err)
	}
	if event.Type != "payment_intent.succeeded" {
		return nil
	}
	var intent stripe.PaymentIntent
	if err := json.Unmarshal(event.Data.Raw, &intent); err != nil {
		return fmt.Errorf("payment: decode payment intent: %w", err)
	}
	if intent.Status != stripe.PaymentIntentStatusSucceeded {
		return nil
	}
	p, err := s.repo.ByProviderRef(intent.ID)
	if err != nil {
		return fmt.Errorf("payment: load by provider ref %s: %w", intent.ID, err)
	}
	if p.Status == StatusPaid {
		return nil // already reconciled (e.g. by Status's fallback) - idempotent
	}
	if !coversPayment(&intent, p) {
		s.log.Error("payment: webhook intent for the wrong amount", "payment", p.ID,
			"intent", intent.ID, "amount", intent.AmountReceived, "currency", intent.Currency)
		return nil // acknowledged, not honoured - retrying would not change it
	}
	p.Status = StatusPaid
	if err := s.repo.Update(p); err != nil {
		return fmt.Errorf("payment: mark paid: %w", err)
	}
	s.log.Info("payment: webhook marked paid", "payment", p.ID, "job", p.JobID)
	return nil
}
