package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"sade/config"
	"sade/internals/app/asset"
	"sade/internals/app/auth"
	"sade/internals/app/job"
	"sade/internals/app/magic_token"
	"sade/internals/app/payment"
	"sade/internals/app/session"
	"sade/internals/app/share"
	"sade/internals/app/user"
	"sade/internals/database"
	"sade/internals/mailer"
	"sade/internals/storage"
	"sade/internals/testutil"
	"sade/internals/token"

	stripe "github.com/stripe/stripe-go/v82"
)

type capMailer struct {
	mu   sync.Mutex
	sent []mailer.Message
}

func (m *capMailer) Send(_ context.Context, msg mailer.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}

func (m *capMailer) lastLink(t *testing.T) string {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	body := m.sent[len(m.sent)-1].Text
	i := strings.Index(body, "http://")
	if i < 0 {
		t.Fatalf("no link in body:\n%s", body)
	}
	link := body[i:]
	if j := strings.IndexAny(link, " \n\r"); j >= 0 {
		link = link[:j]
	}
	return link
}

// testApp bundles the pieces a test may need to reach past the HTTP surface
// (seed a preview asset, sign a share token).
type testApp struct {
	db     *database.Database
	store  storage.Storage
	signer *token.Signer
}

const testShareSecret = "router-test-share-secret-00000000"

// buildTestApp wires the real router over an in-memory DB and a capturing
// mailer, and returns an httptest server, a client with a cookie jar, the
// mailer, and the testApp internals.
func buildTestApp(t *testing.T) (*httptest.Server, *http.Client, *capMailer, *testApp) {
	t.Helper()
	db := testutil.DB(t, user.User{}, magic_token.MagicToken{}, session.Session{}, job.Job{}, asset.Asset{})
	cm := &capMailer{}

	cfg := &config.Config{}
	cfg.Server.CORSAllowedOrigins = []string{"http://localhost:5173"}
	cfg.Auth = config.AuthConfig{
		MagicLinkTTL:      config.Duration(15 * time.Minute),
		SessionTTL:        config.Duration(24 * time.Hour),
		SessionCookieName: "sade_session",
		HMACSecret:        testShareSecret,
	}
	cfg.Upload = config.Defaults().Upload

	store, err := storage.New(config.StorageConfig{Driver: "local", LocalPath: t.TempDir()}, testutil.Logger())
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	signer, err := token.New(cfg.Auth.HMACSecret)
	if err != nil {
		t.Fatalf("token.New: %v", err)
	}

	userSvc := user.NewService(user.NewRepo(db), testutil.Logger())
	authSvc := auth.NewService(
		userSvc, magic_token.NewRepo(db), session.NewRepo(db), cm,
		cfg.Auth, "http://APIBASE", "http://app.test", testutil.Logger(),
	)
	jobSvc := job.NewService(job.NewRepo(db), asset.NewRepo(db), store, cfg.Upload, testutil.Logger())

	// Disabled by default (no Stripe secret key) - tests that need a live
	// payment flow build their own via buildTestAppWithPayment below.
	paymentSvc := payment.NewService(
		payment.NewRepo(db), asset.NewRepo(db), signer, nil, config.PaymentConfig{},
		"http://APIBASE", time.Hour, testutil.Logger(),
	)

	router := NewRouter(Deps{
		Cfg:     cfg,
		Log:     testutil.Logger(),
		Auth:    auth.NewHandler(authSvc, cfg.Auth, testutil.Logger()),
		AuthSvc: authSvc,
		User:    user.NewHandler(userSvc, testutil.Logger()),
		Job:     job.NewHandler(jobSvc, store, cfg.Upload, testutil.Logger()),
		Share:   share.NewHandler(signer, asset.NewRepo(db), store, testutil.Logger()),
		Payment: payment.NewHandler(paymentSvc, testutil.Logger()),
	})

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		// Do not follow the callback's redirect - we want to inspect it.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return srv, client, cm, &testApp{db: db, store: store, signer: signer}
}

func TestAuthFlowEndToEnd(t *testing.T) {
	srv, client, cm, _ := buildTestApp(t)

	// 0. /api/me while signed out -> 401
	if resp := get(t, client, srv.URL+"/api/me"); resp != http.StatusUnauthorized {
		t.Fatalf("/api/me signed out = %d, want 401", resp)
	}

	// 1. request a link
	resp, err := client.Post(srv.URL+"/api/auth/request", "application/json",
		strings.NewReader(`{"email":"op@example.com"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("request link = %d, want 202", resp.StatusCode)
	}

	// 2. follow the callback link from the email; the API base in the link
	// is a placeholder, so swap in the test server's URL.
	link := cm.lastLink(t)
	link = strings.Replace(link, "http://APIBASE", srv.URL, 1)
	cb, err := client.Get(link)
	if err != nil {
		t.Fatal(err)
	}
	cb.Body.Close()
	if cb.StatusCode != http.StatusSeeOther {
		t.Fatalf("callback = %d, want 303", cb.StatusCode)
	}
	if loc := cb.Header.Get("Location"); loc != "http://app.test" {
		t.Errorf("callback Location = %q", loc)
	}
	if !hasSessionCookie(client, srv.URL) {
		t.Fatal("no session cookie in the jar after callback")
	}

	// 3. /api/me now returns the signed-in user
	me, err := client.Get(srv.URL + "/api/me")
	if err != nil {
		t.Fatal(err)
	}
	defer me.Body.Close()
	if me.StatusCode != http.StatusOK {
		t.Fatalf("/api/me signed in = %d, want 200", me.StatusCode)
	}
	var u user.Response
	if err := json.NewDecoder(me.Body).Decode(&u); err != nil {
		t.Fatal(err)
	}
	if u.Email != "op@example.com" || u.Role != user.RoleOperator {
		t.Errorf("/api/me user = %+v", u)
	}

	// 4. an operator cannot reach the admin route
	if resp := get(t, client, srv.URL+"/api/users"); resp != http.StatusForbidden {
		t.Errorf("/api/users as operator = %d, want 403", resp)
	}

	// 5. logout, then /api/me is 401 again
	lo, err := client.Post(srv.URL+"/api/auth/logout", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	lo.Body.Close()
	if lo.StatusCode != http.StatusNoContent {
		t.Fatalf("logout = %d, want 204", lo.StatusCode)
	}
	if resp := get(t, client, srv.URL+"/api/me"); resp != http.StatusUnauthorized {
		t.Errorf("/api/me after logout = %d, want 401", resp)
	}
}

// signIn runs the magic-link dance and leaves the client holding a session
// cookie for email.
func signIn(t *testing.T, srv *httptest.Server, client *http.Client, cm *capMailer, email string) {
	t.Helper()
	resp, err := client.Post(srv.URL+"/api/auth/request", "application/json",
		strings.NewReader(`{"email":"`+email+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("request link = %d, want 202", resp.StatusCode)
	}
	link := strings.Replace(cm.lastLink(t), "http://APIBASE", srv.URL, 1)
	cb, err := client.Get(link)
	if err != nil {
		t.Fatal(err)
	}
	cb.Body.Close()
	if !hasSessionCookie(client, srv.URL) {
		t.Fatal("no session cookie after callback")
	}
}

// multipartUpload builds a POST /api/jobs body with one file part and extra
// text fields, returning the body and its Content-Type header.
func multipartUpload(t *testing.T, filename string, content []byte, fields map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, mw.FormDataContentType()
}

func TestJobUploadFlowEndToEnd(t *testing.T) {
	srv, client, cm, _ := buildTestApp(t)

	// Unauthenticated upload is rejected.
	body, ct := multipartUpload(t, "clip.png", []byte("img-bytes"),
		map[string]string{"recipientEmail": "client@example.com"})
	resp, err := client.Post(srv.URL+"/api/jobs", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("upload signed out = %d, want 401", resp.StatusCode)
	}

	signIn(t, srv, client, cm, "op@example.com")

	// Upload a file -> 201 with a pending job and its original asset.
	body, ct = multipartUpload(t, "clip.png", []byte("img-bytes"), map[string]string{
		"recipientEmail": "client@example.com",
		"watermarkKind":  job.WatermarkText,
		"watermarkText":  "confidential",
	})
	resp, err = client.Post(srv.URL+"/api/jobs", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload = %d, want 201", resp.StatusCode)
	}
	var created job.Response
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Status != job.StatusPending || created.MediaType != job.MediaImage {
		t.Errorf("created job = %+v", created)
	}
	if len(created.Assets) != 1 || created.Assets[0].Kind != asset.KindOriginal || created.Assets[0].SizeBytes == 0 {
		t.Errorf("created assets = %+v", created.Assets)
	}

	// It shows up in the caller's list.
	lresp, err := client.Get(srv.URL + "/api/jobs")
	if err != nil {
		t.Fatal(err)
	}
	defer lresp.Body.Close()
	var list []job.Response
	if err := json.NewDecoder(lresp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list = %+v, want the one created job", list)
	}

	// Detail returns the job with its assets.
	dresp, err := client.Get(srv.URL + "/api/jobs/" + created.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer dresp.Body.Close()
	if dresp.StatusCode != http.StatusOK {
		t.Fatalf("detail = %d, want 200", dresp.StatusCode)
	}
	var detail job.Response
	if err := json.NewDecoder(dresp.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.ID != created.ID || len(detail.Assets) != 1 {
		t.Errorf("detail = %+v", detail)
	}

	// A rejected extension is a 415 and creates nothing.
	body, ct = multipartUpload(t, "notes.txt", []byte("plain"),
		map[string]string{"recipientEmail": "client@example.com"})
	bresp, err := client.Post(srv.URL+"/api/jobs", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	bresp.Body.Close()
	if bresp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("bad-extension upload = %d, want 415", bresp.StatusCode)
	}

	// Another operator does not see the first operator's job.
	jar2, _ := cookiejar.New(nil)
	client2 := &http.Client{Jar: jar2, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	signIn(t, srv, client2, cm, "other@example.com")
	oresp := get(t, client2, srv.URL+"/api/jobs/"+created.ID)
	if oresp != http.StatusNotFound {
		t.Errorf("other operator GET job = %d, want 404", oresp)
	}
}

func TestSharePreviewFlowEndToEnd(t *testing.T) {
	srv, client, cm, ta := buildTestApp(t)

	// An operator uploads a job (no worker runs in the test).
	signIn(t, srv, client, cm, "op@example.com")
	body, ct := multipartUpload(t, "clip.png", []byte("original-bytes"), map[string]string{
		"recipientEmail": "client@example.com",
	})
	resp, err := client.Post(srv.URL+"/api/jobs", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	var created job.Response
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	// Simulate what the worker would produce: a preview blob + asset row.
	previewBytes := []byte("WATERMARKED-PREVIEW-CONTENT-9876543210")
	previewKey := "previews/" + created.ID + "/clip-preview.png"
	if _, err := ta.store.Put(context.Background(), previewKey, bytes.NewReader(previewBytes)); err != nil {
		t.Fatalf("put preview blob: %v", err)
	}
	previewID, err := asset.NewRepo(ta.db).Create(&asset.Asset{
		JobID: created.ID, Kind: asset.KindPreview, StorageKey: previewKey,
		Filename: "clip-preview.png", MIME: "image/png", SizeBytes: int64(len(previewBytes)),
	})
	if err != nil {
		t.Fatalf("create preview asset: %v", err)
	}

	// The public /p link (no cookie needed) streams the preview inline.
	viewTok, _ := ta.signer.Sign("preview", previewID, time.Hour)
	noAuth := &http.Client{}
	pv, err := noAuth.Get(srv.URL + "/p/" + viewTok)
	if err != nil {
		t.Fatal(err)
	}
	defer pv.Body.Close()
	if pv.StatusCode != http.StatusOK {
		t.Fatalf("/p = %d, want 200", pv.StatusCode)
	}
	got, _ := io.ReadAll(pv.Body)
	if !bytes.Equal(got, previewBytes) {
		t.Errorf("/p body = %q", got)
	}
	if cd := pv.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, "inline;") {
		t.Errorf("/p Content-Disposition = %q, want inline", cd)
	}

	// The /d link serves the same bytes as an attachment.
	dlTok, _ := ta.signer.Sign("download", previewID, time.Hour)
	dl, err := noAuth.Get(srv.URL + "/d/" + dlTok)
	if err != nil {
		t.Fatal(err)
	}
	defer dl.Body.Close()
	if dl.StatusCode != http.StatusOK {
		t.Fatalf("/d = %d, want 200", dl.StatusCode)
	}
	if cd := dl.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment;") {
		t.Errorf("/d Content-Disposition = %q, want attachment", cd)
	}

	// A tampered token is a 404.
	if bad := get(t, noAuth, srv.URL+"/p/"+viewTok+"tampered"); bad != http.StatusNotFound {
		t.Errorf("tampered /p token = %d, want 404", bad)
	}
}

func TestJobAssetContentFlowEndToEnd(t *testing.T) {
	srv, client, cm, ta := buildTestApp(t)

	signIn(t, srv, client, cm, "op@example.com")
	body, ct := multipartUpload(t, "clip.png", []byte("original-bytes"), map[string]string{
		"recipientEmail": "client@example.com",
	})
	resp, err := client.Post(srv.URL+"/api/jobs", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	var created job.Response
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	originalID := created.Assets[0].ID

	// The owning operator streams the original inline, authenticated by the
	// session cookie alone - no signed token involved.
	oc, err := client.Get(srv.URL + "/api/jobs/" + created.ID + "/assets/" + originalID + "/content")
	if err != nil {
		t.Fatal(err)
	}
	defer oc.Body.Close()
	if oc.StatusCode != http.StatusOK {
		t.Fatalf("content(original) = %d, want 200", oc.StatusCode)
	}
	if got, _ := io.ReadAll(oc.Body); string(got) != "original-bytes" {
		t.Errorf("content(original) body = %q", got)
	}
	if cd := oc.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, "inline;") {
		t.Errorf("content(original) Content-Disposition = %q, want inline", cd)
	}

	// Simulate what the worker would produce: a preview blob + asset row.
	previewBytes := []byte("WATERMARKED-PREVIEW-CONTENT")
	previewKey := "previews/" + created.ID + "/clip-preview.png"
	if _, err := ta.store.Put(context.Background(), previewKey, bytes.NewReader(previewBytes)); err != nil {
		t.Fatalf("put preview blob: %v", err)
	}
	previewID, err := asset.NewRepo(ta.db).Create(&asset.Asset{
		JobID: created.ID, Kind: asset.KindPreview, StorageKey: previewKey,
		Filename: "clip-preview.png", MIME: "image/png", SizeBytes: int64(len(previewBytes)),
	})
	if err != nil {
		t.Fatalf("create preview asset: %v", err)
	}

	// The same authenticated route also serves the preview (unlike the
	// public /p/{token}, which only ever serves KindPreview).
	pc, err := client.Get(srv.URL + "/api/jobs/" + created.ID + "/assets/" + previewID + "/content")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Body.Close()
	if got, _ := io.ReadAll(pc.Body); string(got) != string(previewBytes) {
		t.Errorf("content(preview) body = %q", got)
	}

	// ?dl=1 asks for an attachment instead of inline.
	dl, err := client.Get(srv.URL + "/api/jobs/" + created.ID + "/assets/" + previewID + "/content?dl=1")
	if err != nil {
		t.Fatal(err)
	}
	dl.Body.Close()
	if cd := dl.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment;") {
		t.Errorf("content?dl=1 Content-Disposition = %q, want attachment", cd)
	}

	// Signed out entirely: 401.
	noAuth := &http.Client{}
	if s := get(t, noAuth, srv.URL+"/api/jobs/"+created.ID+"/assets/"+originalID+"/content"); s != http.StatusUnauthorized {
		t.Errorf("signed-out content = %d, want 401", s)
	}

	// Another operator gets a 404, not the first operator's bytes.
	jar2, _ := cookiejar.New(nil)
	client2 := &http.Client{Jar: jar2, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	signIn(t, srv, client2, cm, "other@example.com")
	if s := get(t, client2, srv.URL+"/api/jobs/"+created.ID+"/assets/"+originalID+"/content"); s != http.StatusNotFound {
		t.Errorf("other operator content = %d, want 404", s)
	}

	// An asset id that exists but under a different job: also 404, not the
	// wrong job's file.
	body, ct = multipartUpload(t, "other.png", []byte("other-original"), map[string]string{
		"recipientEmail": "client2@example.com",
	})
	resp2, err := client.Post(srv.URL+"/api/jobs", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	var created2 job.Response
	json.NewDecoder(resp2.Body).Decode(&created2)
	resp2.Body.Close()
	if s := get(t, client, srv.URL+"/api/jobs/"+created2.ID+"/assets/"+originalID+"/content"); s != http.StatusNotFound {
		t.Errorf("asset under wrong job = %d, want 404", s)
	}
}

// --- payment ---------------------------------------------------------------

// fakePaymentBackend is a minimal stripe.Backend answering payment-intent
// create/retrieve calls with canned JSON - see the identical helper in
// internals/app/payment/service_test.go for why (no network call, no real
// Stripe account needed). Small enough to duplicate rather than export a
// cross-package test double.
type fakePaymentBackend struct {
	intentStatus stripe.PaymentIntentStatus
	n            int
}

func (f *fakePaymentBackend) Call(method, path, _ string, _ stripe.ParamsContainer, v stripe.LastResponseSetter) error {
	var body map[string]any
	switch {
	case method == "POST" && path == "/v1/payment_intents":
		f.n++
		body = map[string]any{
			"id": fmt.Sprintf("pi_test_%d", f.n), "object": "payment_intent",
			"client_secret": fmt.Sprintf("pi_test_%d_secret_fake", f.n), "status": "requires_payment_method",
		}
	case method == "GET":
		body = map[string]any{
			"id": path[len("/v1/payment_intents/"):], "object": "payment_intent",
			"status": string(f.intentStatus),
		}
	default:
		return fmt.Errorf("fakePaymentBackend: unhandled %s %s", method, path)
	}
	b, _ := json.Marshal(body)
	if err := json.Unmarshal(b, v); err != nil {
		return err
	}
	v.SetLastResponse(&stripe.APIResponse{})
	return nil
}
func (f *fakePaymentBackend) CallStreaming(string, string, string, stripe.ParamsContainer, stripe.StreamingLastResponseSetter) error {
	return errors.New("fakePaymentBackend: not implemented")
}
func (f *fakePaymentBackend) CallRaw(string, string, string, []byte, *stripe.Params, stripe.LastResponseSetter) error {
	return errors.New("fakePaymentBackend: not implemented")
}
func (f *fakePaymentBackend) CallMultipart(string, string, string, string, *bytes.Buffer, *stripe.Params, stripe.LastResponseSetter) error {
	return errors.New("fakePaymentBackend: not implemented")
}
func (f *fakePaymentBackend) SetMaxNetworkRetries(int64) {}

// buildTestAppWithPayment is buildTestApp plus a Stripe-enabled payment
// service backed by fakePaymentBackend, so the checkout/status/webhook
// routes and the /o/{token} original download can be exercised end to end.
func buildTestAppWithPayment(t *testing.T) (*httptest.Server, *http.Client, *capMailer, *testApp, *fakePaymentBackend) {
	t.Helper()
	db := testutil.DB(t, user.User{}, magic_token.MagicToken{}, session.Session{}, job.Job{}, asset.Asset{}, payment.Payment{})
	cm := &capMailer{}

	cfg := &config.Config{}
	cfg.Server.CORSAllowedOrigins = []string{"http://localhost:5173"}
	cfg.Auth = config.AuthConfig{
		MagicLinkTTL: config.Duration(15 * time.Minute), SessionTTL: config.Duration(24 * time.Hour),
		SessionCookieName: "sade_session", HMACSecret: testShareSecret,
	}
	cfg.Upload = config.Defaults().Upload

	store, err := storage.New(config.StorageConfig{Driver: "local", LocalPath: t.TempDir()}, testutil.Logger())
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	signer, err := token.New(cfg.Auth.HMACSecret)
	if err != nil {
		t.Fatalf("token.New: %v", err)
	}

	userSvc := user.NewService(user.NewRepo(db), testutil.Logger())
	authSvc := auth.NewService(
		userSvc, magic_token.NewRepo(db), session.NewRepo(db), cm,
		cfg.Auth, "http://APIBASE", "http://app.test", testutil.Logger(),
	)
	jobSvc := job.NewService(job.NewRepo(db), asset.NewRepo(db), store, cfg.Upload, testutil.Logger())

	// Unlike the other test apps, payment.Service.Status mints an /o/{token}
	// URL against its own configured publicURL and this test then actually
	// dereferences that URL - it has to be the real httptest address, not a
	// placeholder. NewUnstartedServer gives us that address before the
	// handler (and so the router, and so paymentSvc) has to exist.
	srv := httptest.NewUnstartedServer(nil)
	publicURL := "http://" + srv.Listener.Addr().String()

	backend := &fakePaymentBackend{}
	stripeClient := stripe.NewClient("sk_test_fake", stripe.WithBackends(&stripe.Backends{API: backend}))
	paymentCfg := config.PaymentConfig{StripeSecretKey: "sk_test_fake", StripeWebhookSecret: "whsec_test", PriceCents: 4900, Currency: "eur"}
	paymentSvc := payment.NewService(
		payment.NewRepo(db), asset.NewRepo(db), signer, stripeClient, paymentCfg,
		publicURL, time.Hour, testutil.Logger(),
	)

	router := NewRouter(Deps{
		Cfg:     cfg,
		Log:     testutil.Logger(),
		Auth:    auth.NewHandler(authSvc, cfg.Auth, testutil.Logger()),
		AuthSvc: authSvc,
		User:    user.NewHandler(userSvc, testutil.Logger()),
		Job:     job.NewHandler(jobSvc, store, cfg.Upload, testutil.Logger()),
		Share:   share.NewHandler(signer, asset.NewRepo(db), store, testutil.Logger()),
		Payment: payment.NewHandler(paymentSvc, testutil.Logger()),
	})

	srv.Config.Handler = router
	srv.Start()
	t.Cleanup(srv.Close)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return srv, client, cm, &testApp{db: db, store: store, signer: signer}, backend
}

func TestPaymentUnlockFlowEndToEnd(t *testing.T) {
	srv, client, cm, ta, backend := buildTestAppWithPayment(t)

	// An operator uploads a job (no worker runs in the test).
	signIn(t, srv, client, cm, "op@example.com")
	body, ct := multipartUpload(t, "clip.png", []byte("clean-original-bytes"), map[string]string{
		"recipientEmail": "client@example.com",
	})
	resp, err := client.Post(srv.URL+"/api/jobs", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	var created job.Response
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	// Simulate what the worker would produce: a preview asset.
	previewBytes := []byte("WATERMARKED-PREVIEW")
	previewKey := "previews/" + created.ID + "/clip-preview.png"
	if _, err := ta.store.Put(context.Background(), previewKey, bytes.NewReader(previewBytes)); err != nil {
		t.Fatalf("put preview blob: %v", err)
	}
	previewID, err := asset.NewRepo(ta.db).Create(&asset.Asset{
		JobID: created.ID, Kind: asset.KindPreview, StorageKey: previewKey,
		Filename: "clip-preview.png", MIME: "image/png", SizeBytes: int64(len(previewBytes)),
	})
	if err != nil {
		t.Fatalf("create preview asset: %v", err)
	}
	previewTok, err := ta.signer.Sign("preview", previewID, time.Hour)
	if err != nil {
		t.Fatalf("sign preview token: %v", err)
	}

	noAuth := &http.Client{}

	// Mirrors the unexported JSON shapes internals/app/payment.Handler
	// writes (payment is a different package; this test only cares about
	// the wire format).
	type paymentStatus struct {
		Enabled     bool   `json:"enabled"`
		Paid        bool   `json:"paid"`
		OriginalURL string `json:"originalUrl"`
	}
	type paymentIntent struct {
		ClientSecret string `json:"clientSecret"`
	}

	// Before paying: status reports enabled, unpaid.
	sresp, err := noAuth.Get(srv.URL + "/api/payments/status?token=" + previewTok)
	if err != nil {
		t.Fatal(err)
	}
	var status paymentStatus
	json.NewDecoder(sresp.Body).Decode(&status)
	sresp.Body.Close()
	if !status.Enabled || status.Paid {
		t.Fatalf("status before paying = %+v", status)
	}

	// Start a payment intent (what the frontend's Stripe Elements form does
	// on mount).
	checkoutBody, _ := json.Marshal(map[string]string{"token": previewTok})
	cresp, err := noAuth.Post(srv.URL+"/api/payments/checkout", "application/json", bytes.NewReader(checkoutBody))
	if err != nil {
		t.Fatal(err)
	}
	if cresp.StatusCode != http.StatusOK {
		t.Fatalf("checkout = %d, want 200", cresp.StatusCode)
	}
	var intent paymentIntent
	json.NewDecoder(cresp.Body).Decode(&intent)
	cresp.Body.Close()
	if intent.ClientSecret == "" {
		t.Fatal("checkout returned an empty client secret")
	}

	// Stripe now reports the payment intent succeeded (as if the recipient
	// completed the Payment Element form) - status should reconcile and
	// hand back a working /o/{token} link, with no webhook involved.
	backend.intentStatus = stripe.PaymentIntentStatusSucceeded
	sresp, err = noAuth.Get(srv.URL + "/api/payments/status?token=" + previewTok)
	if err != nil {
		t.Fatal(err)
	}
	json.NewDecoder(sresp.Body).Decode(&status)
	sresp.Body.Close()
	if !status.Paid || status.OriginalURL == "" {
		t.Fatalf("status after paying = %+v", status)
	}

	oresp, err := noAuth.Get(status.OriginalURL)
	if err != nil {
		t.Fatal(err)
	}
	defer oresp.Body.Close()
	if oresp.StatusCode != http.StatusOK {
		t.Fatalf("GET originalUrl = %d, want 200", oresp.StatusCode)
	}
	got, _ := io.ReadAll(oresp.Body)
	if string(got) != "clean-original-bytes" {
		t.Errorf("original content = %q", got)
	}
	if cd := oresp.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment;") {
		t.Errorf("original Content-Disposition = %q, want attachment", cd)
	}

	// A preview token can never reach the original through /o/ - wrong
	// purpose entirely.
	if s := get(t, noAuth, srv.URL+"/o/"+previewTok); s != http.StatusNotFound {
		t.Errorf("/o/ with a preview token = %d, want 404", s)
	}
}

func TestHealthz(t *testing.T) {
	srv, client, _, _ := buildTestApp(t)
	resp, err := client.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/healthz = %d", resp.StatusCode)
	}
}

func TestCORSPreflight(t *testing.T) {
	srv, client, _, _ := buildTestApp(t)
	req, _ := http.NewRequest(http.MethodOptions, srv.URL+"/api/me", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight = %d, want 204", resp.StatusCode)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Errorf("missing CORS allow-origin: %v", resp.Header)
	}
	if resp.Header.Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("missing allow-credentials")
	}
}

func get(t *testing.T, c *http.Client, url string) int {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func hasSessionCookie(c *http.Client, base string) bool {
	u, _ := http.NewRequest("GET", base, nil)
	for _, ck := range c.Jar.Cookies(u.URL) {
		if ck.Name == "sade_session" && ck.Value != "" {
			return true
		}
	}
	return false
}
