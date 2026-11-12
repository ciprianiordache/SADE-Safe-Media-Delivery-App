package share_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"sade/internals/app/asset"
	"sade/internals/app/job"
	"sade/internals/app/job_event"
	"sade/internals/app/share"
	"sade/internals/testutil"
	"sade/internals/token"
)

// --- fakes ------------------------------------------------------------------

type fakeLinks struct{ off map[string]bool }

func (f *fakeLinks) LinksDisabled(jobID string) (bool, error) { return f.off[jobID], nil }

type fakeEvents struct {
	mu   sync.Mutex
	list []job_event.JobEvent
}

func (f *fakeEvents) Record(jobID, kind, detail string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.list = append([]job_event.JobEvent{{JobID: jobID, Kind: kind, Detail: detail, CreatedAt: time.Now().UTC()}}, f.list...)
	return nil
}

func (f *fakeEvents) RecordOnce(jobID, kind string, window time.Duration) error {
	if n, _ := f.CountSince(jobID, kind, time.Now().UTC().Add(-window)); n > 0 {
		return nil
	}
	return f.Record(jobID, kind, "")
}

func (f *fakeEvents) ListByJob(jobID string) ([]job_event.JobEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []job_event.JobEvent
	for _, e := range f.list {
		if e.JobID == jobID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeEvents) CountSince(jobID, kind string, since time.Time) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, e := range f.list {
		if e.JobID == jobID && e.Kind == kind && !e.CreatedAt.Before(since) {
			n++
		}
	}
	return n, nil
}

func (f *fakeEvents) count(kind string) int {
	n, _ := f.CountSince("job1", kind, time.Time{})
	return n
}

type fakeJobs struct{ j job.Response }

func (f fakeJobs) Get(userID, jobID string) (job.Response, error) {
	if userID != "owner" || jobID != f.j.ID {
		return job.Response{}, job.ErrNotFound
	}
	return f.j, nil
}

type fakeFlags struct {
	links *fakeLinks
	paid  bool
}

func (f fakeFlags) SetLinksDisabled(jobID string, d bool) error {
	f.links.off[jobID] = d
	return nil
}
func (f fakeFlags) HasPaidPayment(string) (bool, error) { return f.paid, nil }

type fakeNotifier struct{ sent []string }

func (f *fakeNotifier) PreviewReady(_ context.Context, recipient, previewID string) error {
	f.sent = append(f.sent, recipient+"|"+previewID)
	return nil
}

// --- public links -----------------------------------------------------------

func TestPublicLinksHonourTheSwitchAndRecordAccess(t *testing.T) {
	store := localStore(t)
	pa := &asset.Asset{ID: "prev1", JobID: "job1", Kind: asset.KindPreview, StorageKey: "previews/job1/p.mp4", Filename: "p.mp4", MIME: "video/mp4"}
	if _, err := store.Put(context.Background(), pa.StorageKey, bytes.NewReader([]byte(strings.Repeat("x", 100)))); err != nil {
		t.Fatal(err)
	}
	signer, _ := token.New(testSecret)
	links := &fakeLinks{off: map[string]bool{}}
	events := &fakeEvents{}
	h := share.NewHandler(signer, fakeAssets{byID: map[string]*asset.Asset{"prev1": pa}}, store, testutil.Logger())
	h.UseDelivery(links, events)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /p/{token}", h.Preview)
	mux.HandleFunc("GET /d/{token}", h.Download)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	view, _ := signer.Sign("preview", "prev1", time.Hour)
	get := func(path, rng string) int {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		if rng != "" {
			req.Header.Set("Range", rng)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	// A player's burst of range requests is one "opened".
	get("/p/"+view, "bytes=0-")
	get("/p/"+view, "bytes=50-")
	get("/p/"+view, "bytes=0-")
	if n := events.count(job_event.KindViewed); n != 1 {
		t.Errorf("viewed events = %d, want 1", n)
	}

	links.off["job1"] = true
	if code := get("/p/"+view, ""); code != http.StatusGone {
		t.Errorf("disabled preview = %d, want 410", code)
	}
	dl, _ := signer.Sign("download", "prev1", time.Hour)
	if code := get("/d/"+dl, ""); code != http.StatusGone {
		t.Errorf("disabled download = %d, want 410", code)
	}
	if n := events.count(job_event.KindDownloaded); n != 0 {
		t.Errorf("a refused download was recorded")
	}
}

// --- operator routes --------------------------------------------------------

func operatorHarness(t *testing.T, paid bool) (*share.OperatorHandler, *fakeLinks, *fakeEvents, *fakeNotifier) {
	t.Helper()
	signer, _ := token.New(testSecret)
	links := &fakeLinks{off: map[string]bool{}}
	events := &fakeEvents{}
	n := &fakeNotifier{}
	j := job.Response{ID: "job1", Status: "done", RecipientEmail: "client@example.com",
		Assets: []asset.Response{{ID: "orig1", Kind: asset.KindOriginal}, {ID: "prev1", Kind: asset.KindPreview}}}
	h := share.NewOperatorHandler(fakeJobs{j: j}, fakeFlags{links: links, paid: paid}, events, n,
		signer, "http://app.test", time.Hour, testutil.Logger())
	return h, links, events, n
}

func call(h http.HandlerFunc, method, userID, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "/api/jobs/job1/x", strings.NewReader(body))
	r.SetPathValue("id", "job1")
	if userID != "" {
		r = r.WithContext(job.WithUserID(r.Context(), userID))
	}
	w := httptest.NewRecorder()
	h(w, r)
	return w
}

func TestDeliveryIsOwnerScopedAndCarriesALink(t *testing.T) {
	h, _, _, _ := operatorHarness(t, false)
	if w := call(h.Delivery, http.MethodGet, "intruder", ""); w.Code != http.StatusNotFound {
		t.Fatalf("someone else's job = %d, want 404", w.Code)
	}
	w := call(h.Delivery, http.MethodGet, "owner", "")
	var d share.Delivery
	_ = json.Unmarshal(w.Body.Bytes(), &d)
	if w.Code != http.StatusOK || !d.Enabled || !d.Ready || !strings.HasPrefix(d.PreviewURL, "http://app.test/preview/") {
		t.Fatalf("delivery = %d %+v", w.Code, d)
	}
}

func TestDeliveryReportsThePayment(t *testing.T) {
	// Payments off: nothing to show.
	h, _, _, _ := operatorHarness(t, false)
	var d share.Delivery
	_ = json.Unmarshal(call(h.Delivery, http.MethodGet, "owner", "").Body.Bytes(), &d)
	if d.Payments || d.AmountCents != 0 {
		t.Fatalf("payments off = %+v", d)
	}

	// On and unpaid: the price, no paid date.
	h.SetPayment(4900, "eur")
	d = share.Delivery{}
	_ = json.Unmarshal(call(h.Delivery, http.MethodGet, "owner", "").Body.Bytes(), &d)
	if !d.Payments || d.AmountCents != 4900 || d.Currency != "eur" || !d.PaidAt.IsZero() {
		t.Fatalf("unpaid = %+v", d)
	}

	// Paid: the date of the "paid" event.
	h, _, events, _ := operatorHarness(t, true)
	h.SetPayment(4900, "eur")
	_ = events.Record("job1", job_event.KindPaid, "")
	d = share.Delivery{}
	_ = json.Unmarshal(call(h.Delivery, http.MethodGet, "owner", "").Body.Bytes(), &d)
	if !d.Paid || d.PaidAt.IsZero() {
		t.Fatalf("paid = %+v", d)
	}
}

func TestResendIsLimited(t *testing.T) {
	h, _, events, n := operatorHarness(t, false)
	for i := 0; i < 3; i++ {
		if w := call(h.Resend, http.MethodPost, "owner", ""); w.Code != http.StatusNoContent {
			t.Fatalf("resend #%d = %d", i+1, w.Code)
		}
	}
	if w := call(h.Resend, http.MethodPost, "owner", ""); w.Code != http.StatusTooManyRequests {
		t.Errorf("4th resend within the hour = %d, want 429", w.Code)
	}
	if len(n.sent) != 3 || n.sent[0] != "client@example.com|prev1" {
		t.Errorf("emails = %v", n.sent)
	}
	if c := events.count(job_event.KindResent); c != 3 {
		t.Errorf("resent events = %d", c)
	}
}

func TestLinksSwitch(t *testing.T) {
	h, links, events, _ := operatorHarness(t, false)
	if w := call(h.SetLinks, http.MethodPut, "owner", `{"enabled":false}`); w.Code != http.StatusNoContent {
		t.Fatalf("disable = %d", w.Code)
	}
	if !links.off["job1"] || events.count(job_event.KindLinksDisabled) != 1 {
		t.Fatalf("links not switched off / not recorded")
	}

	paidH, _, _, _ := operatorHarness(t, true)
	if w := call(paidH.SetLinks, http.MethodPut, "owner", `{"enabled":false}`); w.Code != http.StatusConflict {
		t.Errorf("disable after payment = %d, want 409", w.Code)
	}
}
