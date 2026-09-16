package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sade/internals/app/asset"
	"sade/internals/app/job"
	"sade/internals/ffmpeg"
	"sade/internals/testutil"
	"sade/internals/worker"
)

// jobStoreAdapter / assetStoreAdapter mirror the unexported adapters in root
// worker_wiring.go (package main, unreachable from an internals/app test) -
// duplicated here, minimally, so this test can run a real worker.Pool against
// the same DB and storage the HTTP layer already uses.

type testJobStoreAdapter struct{ repo job.Repo }

func (a testJobStoreAdapter) ClaimPending(_ context.Context, limit int, now time.Time) ([]worker.Job, error) {
	rows, err := a.repo.ClaimPending(limit, now)
	if err != nil {
		return nil, err
	}
	out := make([]worker.Job, len(rows))
	for i, r := range rows {
		out[i] = worker.Job{
			ID: r.ID, UserID: r.UserID, MediaType: r.MediaType, RecipientEmail: r.RecipientEmail,
			WatermarkKind: r.WatermarkKind, WatermarkText: r.WatermarkText, WatermarkOpts: r.WatermarkOpts,
			Attempts: r.Attempts,
		}
	}
	return out, nil
}
func (a testJobStoreAdapter) MarkDone(_ context.Context, id string) error { return a.repo.MarkDone(id) }
func (a testJobStoreAdapter) MarkFailed(_ context.Context, id, errMsg string) error {
	return a.repo.MarkFailed(id, errMsg)
}
func (a testJobStoreAdapter) MarkForRetry(_ context.Context, id, errMsg string, next time.Time) error {
	return a.repo.MarkForRetry(id, errMsg, next)
}
func (a testJobStoreAdapter) ResetStuck(_ context.Context, cutoff time.Time) (int, error) {
	return a.repo.ResetStuck(cutoff)
}

type testAssetStoreAdapter struct{ repo asset.Repo }

func (a testAssetStoreAdapter) Original(_ context.Context, jobID string) (worker.Original, error) {
	row, err := a.repo.GetByJobAndKind(jobID, asset.KindOriginal)
	if err != nil {
		return worker.Original{}, err
	}
	return worker.Original{StorageKey: row.StorageKey, Filename: row.Filename}, nil
}
func (a testAssetStoreAdapter) AddPreview(_ context.Context, p worker.PreviewInput) (string, error) {
	return a.repo.Create(&asset.Asset{
		JobID: p.JobID, Kind: asset.KindPreview, StorageKey: p.StorageKey,
		Filename: p.Filename, MIME: p.MIME, SizeBytes: p.SizeBytes, Checksum: p.Checksum,
	})
}

// fakeWatermarkEngine stands in for ffmpeg: it never shells out, just writes
// fixed bytes to the requested output path, so this test needs no ffmpeg/
// ffprobe binaries (mirrors internals/worker's own fakeEngine).
type fakeWatermarkEngine struct{ media string }

func (f *fakeWatermarkEngine) Probe(context.Context, string) (*ffmpeg.ProbeResult, error) {
	return &ffmpeg.ProbeResult{Media: f.media}, nil
}
func (f *fakeWatermarkEngine) Watermark(_ context.Context, req ffmpeg.Request) error {
	if err := os.MkdirAll(filepath.Dir(req.OutputPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(req.OutputPath, []byte("WATERMARKED-BY-FAKE-ENGINE"), 0o644)
}

// linksFromBody pulls every http:// link out of an email body, in order (the
// EmailNotifier writes "View it: .../preview/<tok>" then "Download it:
// .../d/<tok>").
func linksFromBody(body string) []string {
	var links []string
	for _, line := range strings.Split(body, "\n") {
		i := strings.Index(line, "http://")
		if i < 0 {
			continue
		}
		link := line[i:]
		if j := strings.IndexAny(link, " \r"); j >= 0 {
			link = link[:j]
		}
		links = append(links, link)
	}
	return links
}

// TestWorkerFlowEndToEndThroughRouter is the one full-flow test that runs the
// actual worker.Pool (not a seeded stand-in, unlike TestSharePreviewFlowEndToEnd):
// an operator uploads a file through the real router, a real worker.Pool
// claims and "watermarks" it (via fakeWatermarkEngine, so no ffmpeg binary is
// needed), and the resulting preview is fetched back through the public
// /p/{token} and /d/{token} routes using the links the notifier actually
// emailed - exercising the full pending -> processing -> done state machine
// plus the claim/notify wiring end to end.
func TestWorkerFlowEndToEndThroughRouter(t *testing.T) {
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
	if created.Status != job.StatusPending {
		t.Fatalf("created job status = %q, want pending", created.Status)
	}

	pool := worker.New(
		worker.Config{
			Concurrency: 1, PollInterval: 10 * time.Millisecond, ClaimBatchSize: 1,
			MaxRetries: 0, RetryBackoff: time.Second, JobTimeout: 5 * time.Second,
			StuckJobTimeout: time.Hour, ShutdownGrace: time.Second,
		},
		testJobStoreAdapter{job.NewRepo(ta.db)},
		testAssetStoreAdapter{asset.NewRepo(ta.db)},
		ta.store,
		&fakeWatermarkEngine{media: job.MediaImage},
		worker.NewEmailNotifier(cm, ta.signer, srv.URL, time.Hour),
		testutil.Logger(),
	)
	ctx, cancel := context.WithCancel(context.Background())
	pool.Start(ctx)
	t.Cleanup(func() {
		cancel()
		stopCtx, sc := context.WithTimeout(context.Background(), 2*time.Second)
		defer sc()
		_ = pool.Stop(stopCtx)
	})

	// Poll the job detail route (the same one the dashboard polls) until the
	// worker has driven it to "done".
	var detail job.Response
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		dresp, err := client.Get(srv.URL + "/api/jobs/" + created.ID)
		if err != nil {
			t.Fatal(err)
		}
		_ = json.NewDecoder(dresp.Body).Decode(&detail)
		dresp.Body.Close()
		if detail.Status == job.StatusDone || detail.Status == job.StatusFailed {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if detail.Status != job.StatusDone {
		t.Fatalf("job status after worker ran = %q, want done (detail=%+v)", detail.Status, detail)
	}
	if len(detail.Assets) != 2 {
		t.Fatalf("expected original+preview assets, got %+v", detail.Assets)
	}

	// The notifier really emailed the recipient a working view (frontend
	// page) + download link. There's no frontend build in this test harness
	// to render the view link itself, so verify its token is genuine by
	// resolving it against the same public /p/{token} route the frontend
	// page would call, then fetch the download link directly.
	cm.mu.Lock()
	lastBody := cm.sent[len(cm.sent)-1].Text
	cm.mu.Unlock()
	links := linksFromBody(lastBody)
	if len(links) != 2 {
		t.Fatalf("expected 2 links (view, download) in notifier email, got %v", links)
	}
	viewLink, dlLink := links[0], links[1]

	viewPrefix := srv.URL + "/preview/"
	if !strings.HasPrefix(viewLink, viewPrefix) {
		t.Fatalf("view link = %q, want prefix %q", viewLink, viewPrefix)
	}
	viewTok := strings.TrimPrefix(viewLink, viewPrefix)

	anon := &http.Client{}
	for _, link := range []string{srv.URL + "/p/" + viewTok, dlLink} {
		r, err := anon.Get(link)
		if err != nil {
			t.Fatalf("GET %s: %v", link, err)
		}
		got, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if r.StatusCode != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", link, r.StatusCode)
		}
		if string(got) != "WATERMARKED-BY-FAKE-ENGINE" {
			t.Errorf("GET %s body = %q, want the fake engine's output", link, got)
		}
	}
}
