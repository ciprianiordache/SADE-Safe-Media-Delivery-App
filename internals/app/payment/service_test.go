package payment

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"sade/config"
	"sade/internals/app/asset"
	"sade/internals/app/job"
	"sade/internals/app/user"
	"sade/internals/database"
	"sade/internals/testutil"
	"sade/internals/token"

	stripe "github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"
)

// --- fake Stripe backend ---------------------------------------------------

// fakeStripeBackend answers checkout-session create/retrieve calls with
// canned JSON, so payment.Service can be exercised without a network call or
// a real Stripe account. It fills v the same way the real backend does:
// unmarshal the JSON body into it, then SetLastResponse.
type fakeStripeBackend struct {
	paymentStatus stripe.CheckoutSessionPaymentStatus // what Retrieve reports
	createErr     error
	sessions      int
}

func (f *fakeStripeBackend) Call(method, path, _ string, _ stripe.ParamsContainer, v stripe.LastResponseSetter) error {
	var body map[string]any
	switch {
	case method == "POST" && path == "/v1/checkout/sessions":
		if f.createErr != nil {
			return f.createErr
		}
		f.sessions++
		body = map[string]any{
			"id": fmt.Sprintf("cs_test_%d", f.sessions), "object": "checkout.session",
			"url":            fmt.Sprintf("https://checkout.stripe.test/%d", f.sessions),
			"payment_status": "unpaid",
		}
	case method == "GET":
		body = map[string]any{
			"id": path[len("/v1/checkout/sessions/"):], "object": "checkout.session",
			"payment_status": string(f.paymentStatus),
		}
	default:
		return fmt.Errorf("fakeStripeBackend: unhandled %s %s", method, path)
	}
	b, _ := json.Marshal(body)
	if err := json.Unmarshal(b, v); err != nil {
		return err
	}
	v.SetLastResponse(&stripe.APIResponse{})
	return nil
}

func (f *fakeStripeBackend) CallStreaming(string, string, string, stripe.ParamsContainer, stripe.StreamingLastResponseSetter) error {
	return errors.New("fakeStripeBackend: CallStreaming not implemented")
}
func (f *fakeStripeBackend) CallRaw(string, string, string, []byte, *stripe.Params, stripe.LastResponseSetter) error {
	return errors.New("fakeStripeBackend: CallRaw not implemented")
}
func (f *fakeStripeBackend) CallMultipart(string, string, string, string, *bytes.Buffer, *stripe.Params, stripe.LastResponseSetter) error {
	return errors.New("fakeStripeBackend: CallMultipart not implemented")
}
func (f *fakeStripeBackend) SetMaxNetworkRetries(int64) {}

// --- fixture -----------------------------------------------------------

// paymentFixture is one job with a preview + original asset pair, ready to
// pay for.
type paymentFixture struct {
	svc        Service
	repo       Repo
	db         *database.Database
	signer     *token.Signer
	previewTok string
	originalID string
	jobID      string
}

func newFixture(t *testing.T, backend stripe.Backend, cfg config.PaymentConfig) *paymentFixture {
	t.Helper()
	db := testutil.DB(t, user.User{}, job.Job{}, asset.Asset{}, Payment{})

	uid, err := db.CRUD().Create(&user.User{Email: "op@example.com"})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	jobID, err := db.CRUD().Create(&job.Job{
		UserID: uid, Status: job.StatusDone, MediaType: job.MediaVideo, RecipientEmail: "client@example.com",
	})
	if err != nil {
		t.Fatalf("seed job: %v", err)
	}

	assets := asset.NewRepo(db)
	originalID, err := assets.Create(&asset.Asset{
		JobID: jobID, Kind: asset.KindOriginal, StorageKey: "originals/" + jobID + "/original.mp4",
		Filename: "clip.mp4", MIME: "video/mp4", SizeBytes: 100,
	})
	if err != nil {
		t.Fatalf("seed original asset: %v", err)
	}
	previewID, err := assets.Create(&asset.Asset{
		JobID: jobID, Kind: asset.KindPreview, StorageKey: "previews/" + jobID + "/clip-preview.mp4",
		Filename: "clip-preview.mp4", MIME: "video/mp4", SizeBytes: 80,
	})
	if err != nil {
		t.Fatalf("seed preview asset: %v", err)
	}

	signer, err := token.New("test-signing-secret-at-least-32-bytes")
	if err != nil {
		t.Fatalf("token.New: %v", err)
	}
	previewTok, err := signer.Sign("preview", previewID, time.Hour)
	if err != nil {
		t.Fatalf("sign preview token: %v", err)
	}

	var client *stripe.Client
	if backend != nil {
		client = stripe.NewClient("sk_test_fake", stripe.WithBackends(&stripe.Backends{API: backend}))
	}

	repo := NewRepo(db)
	svc := NewService(repo, assets, signer, client, cfg, "http://api.test", "http://app.test", time.Hour, testutil.Logger())
	return &paymentFixture{
		svc: svc, repo: repo, db: db, signer: signer,
		previewTok: previewTok, originalID: originalID, jobID: jobID,
	}
}

func testCfg(overrides ...func(*config.PaymentConfig)) config.PaymentConfig {
	cfg := config.PaymentConfig{StripeSecretKey: "sk_test_fake", PriceCents: 4900, Currency: "eur"}
	for _, o := range overrides {
		o(&cfg)
	}
	return cfg
}

// --- tests ---------------------------------------------------------------

func TestCheckoutDisabledWithoutStripeKey(t *testing.T) {
	fx := newFixture(t, nil, config.PaymentConfig{}) // no StripeSecretKey
	if _, err := fx.svc.Checkout(context.Background(), fx.previewTok); !errors.Is(err, ErrDisabled) {
		t.Fatalf("Checkout(disabled) = %v, want ErrDisabled", err)
	}
}

func TestCheckoutRejectsInvalidToken(t *testing.T) {
	fx := newFixture(t, &fakeStripeBackend{}, testCfg())
	if _, err := fx.svc.Checkout(context.Background(), "not-a-real-token"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("Checkout(bad token) = %v, want ErrInvalidToken", err)
	}
}

func TestCheckoutCreatesPendingPaymentAndSession(t *testing.T) {
	backend := &fakeStripeBackend{}
	fx := newFixture(t, backend, testCfg())

	url, err := fx.svc.Checkout(context.Background(), fx.previewTok)
	if err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if url == "" {
		t.Fatal("Checkout returned an empty URL")
	}

	p, err := fx.repo.LatestByJob(fx.jobID)
	if err != nil {
		t.Fatalf("LatestByJob: %v", err)
	}
	if p.Status != StatusPending || p.ProviderRef == "" || p.AmountCents != 4900 {
		t.Errorf("payment row = %+v", p)
	}
}

func TestStatusReportsUnpaidThenReconcilesOnceStripeConfirms(t *testing.T) {
	backend := &fakeStripeBackend{paymentStatus: stripe.CheckoutSessionPaymentStatusUnpaid}
	fx := newFixture(t, backend, testCfg())

	if _, err := fx.svc.Checkout(context.Background(), fx.previewTok); err != nil {
		t.Fatalf("Checkout: %v", err)
	}

	res, err := fx.svc.Status(context.Background(), fx.previewTok)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !res.Enabled || res.Paid || res.OriginalURL != "" {
		t.Errorf("Status(unpaid) = %+v", res)
	}

	// Stripe now reports the Checkout Session as paid (as if the recipient
	// completed it) - Status should reconcile without a webhook.
	backend.paymentStatus = stripe.CheckoutSessionPaymentStatusPaid
	res, err = fx.svc.Status(context.Background(), fx.previewTok)
	if err != nil {
		t.Fatalf("Status (after pay): %v", err)
	}
	if !res.Paid || res.OriginalURL == "" {
		t.Fatalf("Status(paid) = %+v, want Paid with an OriginalURL", res)
	}
	subject, err := fx.signer.Verify("original", res.OriginalURL[len("http://api.test/o/"):])
	if err != nil {
		t.Fatalf("OriginalURL token does not verify: %v", err)
	}
	if subject != fx.originalID {
		t.Errorf("OriginalURL token subject = %q, want %q", subject, fx.originalID)
	}

	p, err := fx.repo.LatestByJob(fx.jobID)
	if err != nil {
		t.Fatalf("LatestByJob: %v", err)
	}
	if p.Status != StatusPaid {
		t.Errorf("payment row status = %q, want paid", p.Status)
	}
}

func TestStatusOnJobWithNoPaymentYet(t *testing.T) {
	fx := newFixture(t, &fakeStripeBackend{}, testCfg())
	res, err := fx.svc.Status(context.Background(), fx.previewTok)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !res.Enabled || res.Paid || res.AmountCents != 4900 {
		t.Errorf("Status(no payment) = %+v", res)
	}
}

func TestWebhookDisabledWithoutSigningSecret(t *testing.T) {
	fx := newFixture(t, &fakeStripeBackend{}, testCfg()) // no StripeWebhookSecret
	err := fx.svc.Webhook(context.Background(), []byte(`{}`), "t=0,v1=deadbeef")
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("Webhook(no secret) = %v, want ErrDisabled", err)
	}
}

func TestWebhookMarksPaymentPaid(t *testing.T) {
	const whSecret = "whsec_test_secret"
	fx := newFixture(t, &fakeStripeBackend{}, testCfg(func(c *config.PaymentConfig) {
		c.StripeWebhookSecret = whSecret
	}))

	if _, err := fx.svc.Checkout(context.Background(), fx.previewTok); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	p, err := fx.repo.LatestByJob(fx.jobID)
	if err != nil {
		t.Fatalf("LatestByJob: %v", err)
	}

	payload := []byte(fmt.Sprintf(
		`{"id":"evt_1","object":"event","type":"checkout.session.completed","data":{"object":{"id":%q,"object":"checkout.session","payment_status":"paid"}}}`,
		p.ProviderRef,
	))
	now := time.Now()
	sig := webhook.ComputeSignature(now, payload, whSecret)
	header := fmt.Sprintf("t=%d,v1=%s", now.Unix(), hex.EncodeToString(sig))

	if err := fx.svc.Webhook(context.Background(), payload, header); err != nil {
		t.Fatalf("Webhook: %v", err)
	}

	p, err = fx.repo.LatestByJob(fx.jobID)
	if err != nil {
		t.Fatalf("LatestByJob (after webhook): %v", err)
	}
	if p.Status != StatusPaid {
		t.Errorf("payment status after webhook = %q, want paid", p.Status)
	}

	// Idempotent: a second delivery of the same event does not error.
	if err := fx.svc.Webhook(context.Background(), payload, header); err != nil {
		t.Errorf("Webhook (replayed) = %v, want nil (idempotent)", err)
	}
}

func TestWebhookRejectsBadSignature(t *testing.T) {
	fx := newFixture(t, &fakeStripeBackend{}, testCfg(func(c *config.PaymentConfig) {
		c.StripeWebhookSecret = "whsec_test_secret"
	}))
	err := fx.svc.Webhook(context.Background(), []byte(`{}`), "t=0,v1=not-a-real-signature")
	if err == nil {
		t.Fatal("Webhook(bad signature) succeeded, want an error")
	}
}
