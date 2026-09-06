package worker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"sade/config"
	"sade/internals/ffmpeg"
	"sade/internals/storage"
	"sade/internals/testutil"
)

// --- fakes ---------------------------------------------------------------

type fakeJobStore struct {
	mu       sync.Mutex
	pending  []Job // handed out once, then drained
	handedID map[string]bool

	done       []string
	failed     map[string]string
	retried    map[string]time.Time
	retryErr   map[string]string
	resetCalls []time.Time
}

func newFakeJobStore(pending ...Job) *fakeJobStore {
	return &fakeJobStore{
		pending:  pending,
		handedID: map[string]bool{},
		failed:   map[string]string{},
		retried:  map[string]time.Time{},
		retryErr: map[string]string{},
	}
}

func (f *fakeJobStore) ClaimPending(_ context.Context, limit int, _ time.Time) ([]Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pending) == 0 {
		return nil, nil
	}
	n := limit
	if n > len(f.pending) {
		n = len(f.pending)
	}
	batch := append([]Job(nil), f.pending[:n]...)
	f.pending = f.pending[n:]
	for _, j := range batch {
		f.handedID[j.ID] = true
	}
	return batch, nil
}

func (f *fakeJobStore) MarkDone(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.done = append(f.done, id)
	return nil
}

func (f *fakeJobStore) MarkFailed(_ context.Context, id, errMsg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failed[id] = errMsg
	return nil
}

func (f *fakeJobStore) MarkForRetry(_ context.Context, id, errMsg string, next time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retried[id] = next
	f.retryErr[id] = errMsg
	return nil
}

func (f *fakeJobStore) ResetStuck(_ context.Context, cutoff time.Time) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resetCalls = append(f.resetCalls, cutoff)
	return 0, nil
}

func (f *fakeJobStore) doneSet() map[string]bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]bool{}
	for _, id := range f.done {
		out[id] = true
	}
	return out
}

type fakeAssets struct {
	mu       sync.Mutex
	original Original
	origErr  error
	added    []PreviewInput
	addErr   error
}

func (f *fakeAssets) Original(_ context.Context, _ string) (Original, error) {
	if f.origErr != nil {
		return Original{}, f.origErr
	}
	return f.original, nil
}

func (f *fakeAssets) AddPreview(_ context.Context, p PreviewInput) (string, error) {
	if f.addErr != nil {
		return "", f.addErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.added = append(f.added, p)
	return "preview-" + p.JobID, nil
}

func (f *fakeAssets) lastPreview() (PreviewInput, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.added) == 0 {
		return PreviewInput{}, false
	}
	return f.added[len(f.added)-1], true
}

type fakeEngine struct {
	probe     *ffmpeg.ProbeResult
	probeErr  error
	watermErr error
	output    string // bytes written to req.OutputPath by Watermark
	lastReq   ffmpeg.Request
}

func (f *fakeEngine) Probe(_ context.Context, _ string) (*ffmpeg.ProbeResult, error) {
	if f.probeErr != nil {
		return nil, f.probeErr
	}
	return f.probe, nil
}

func (f *fakeEngine) Watermark(_ context.Context, req ffmpeg.Request) error {
	f.lastReq = req
	if f.watermErr != nil {
		return f.watermErr
	}
	body := f.output
	if body == "" {
		body = "PREVIEW-BYTES"
	}
	// Mirror the real engine, which creates the output directory.
	if err := os.MkdirAll(filepath.Dir(req.OutputPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(req.OutputPath, []byte(body), 0o644)
}

type fakeNotifier struct {
	mu       sync.Mutex
	calls    []string // "recipient|previewID"
	err      error
	returned chan struct{}
}

func (f *fakeNotifier) PreviewReady(_ context.Context, recipient, previewID string) error {
	f.mu.Lock()
	f.calls = append(f.calls, recipient+"|"+previewID)
	f.mu.Unlock()
	if f.returned != nil {
		select {
		case f.returned <- struct{}{}:
		default:
		}
	}
	return f.err
}

func (f *fakeNotifier) call() (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return "", false
	}
	return f.calls[len(f.calls)-1], true
}

// --- helpers ------------------------------------------------------------

func testConfig() Config {
	return Config{
		Concurrency:     2,
		PollInterval:    10 * time.Millisecond,
		ClaimBatchSize:  4,
		MaxRetries:      2,
		RetryBackoff:    30 * time.Second,
		JobTimeout:      5 * time.Second,
		StuckJobTimeout: time.Hour,
		ShutdownGrace:   2 * time.Second,
		TextTemplate:    "{recipient} · {date}",
	}
}

func localStore(t *testing.T) storage.Storage {
	t.Helper()
	s, err := storage.New(config.StorageConfig{Driver: "local", LocalPath: t.TempDir()}, testutil.Logger())
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	return s
}

func eventually(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met within 2s")
}

// --- tests ------------------------------------------------------------

func TestProcessWritesPreviewAndNotifies(t *testing.T) {
	store := localStore(t)
	origKey := "originals/job1/original.mov"
	if _, err := store.Put(context.Background(), origKey, strings.NewReader("SOURCE")); err != nil {
		t.Fatalf("seed original: %v", err)
	}

	assets := &fakeAssets{original: Original{StorageKey: origKey, Filename: "holiday.mov"}}
	engine := &fakeEngine{probe: &ffmpeg.ProbeResult{Media: ffmpeg.MediaVideo, DurationSec: 12}, output: "WATERMARKED"}
	notifier := &fakeNotifier{}
	p := New(testConfig(), newFakeJobStore(), assets, store, engine, notifier, testutil.Logger())

	j := Job{ID: "job1", MediaType: "video", RecipientEmail: "client@example.com", WatermarkKind: "both", Attempts: 1}
	if err := p.process(context.Background(), j); err != nil {
		t.Fatalf("process: %v", err)
	}

	prev, ok := assets.lastPreview()
	if !ok {
		t.Fatal("no preview asset recorded")
	}
	if prev.StorageKey != "previews/job1/holiday-preview.mp4" {
		t.Errorf("preview key = %q", prev.StorageKey)
	}
	if prev.SizeBytes != int64(len("WATERMARKED")) || prev.Checksum == "" {
		t.Errorf("preview metadata = %+v", prev)
	}
	if prev.MIME != "video/mp4" {
		t.Errorf("preview MIME = %q, want video/mp4", prev.MIME)
	}

	// The blob really landed in storage under that key.
	if fi, err := store.Stat(context.Background(), prev.StorageKey); err != nil || fi.Size == 0 {
		t.Errorf("preview not in storage: fi=%+v err=%v", fi, err)
	}

	got, ok := notifier.call()
	if !ok || got != "client@example.com|preview-job1" {
		t.Errorf("notifier call = %q", got)
	}

	// The engine got a sensible request.
	if engine.lastReq.Media != ffmpeg.MediaVideo || engine.lastReq.Overlay != "both" {
		t.Errorf("engine req = %+v", engine.lastReq)
	}
	if !strings.Contains(engine.lastReq.Text, "client@example.com") {
		t.Errorf("rendered text = %q, want the recipient from the template", engine.lastReq.Text)
	}
}

func TestProcessSurfacesMissingOriginal(t *testing.T) {
	p := New(testConfig(),
		newFakeJobStore(),
		&fakeAssets{origErr: errors.New("not found")},
		localStore(t),
		&fakeEngine{probe: &ffmpeg.ProbeResult{Media: ffmpeg.MediaVideo}},
		&fakeNotifier{}, testutil.Logger())

	if err := p.process(context.Background(), Job{ID: "x"}); err == nil {
		t.Fatal("process returned nil for a missing original")
	}
}

func TestProcessDoesNotFailJobWhenEmailFails(t *testing.T) {
	store := localStore(t)
	origKey := "originals/j/original.png"
	_, _ = store.Put(context.Background(), origKey, strings.NewReader("S"))
	assets := &fakeAssets{original: Original{StorageKey: origKey, Filename: "pic.png"}}
	engine := &fakeEngine{probe: &ffmpeg.ProbeResult{Media: ffmpeg.MediaImage}}
	notifier := &fakeNotifier{err: errors.New("smtp down")}

	p := New(testConfig(), newFakeJobStore(), assets, store, engine, notifier, testutil.Logger())
	if err := p.process(context.Background(), Job{ID: "j", MediaType: "image", WatermarkKind: "logo", RecipientEmail: "c@e.com"}); err != nil {
		t.Fatalf("process should swallow a mail error, got %v", err)
	}
	if _, ok := assets.lastPreview(); !ok {
		t.Error("preview asset should still be recorded")
	}
}

func TestHandleDrivesRetryThenPermanentFailure(t *testing.T) {
	mkPool := func(js *fakeJobStore) *Pool {
		return New(testConfig(), js,
			&fakeAssets{origErr: errors.New("boom")}, // forces process() to error
			localStore(t),
			&fakeEngine{probe: &ffmpeg.ProbeResult{Media: ffmpeg.MediaVideo}},
			&fakeNotifier{}, testutil.Logger())
	}

	// Attempt 1 of 2 -> reschedule with ~1x backoff.
	js := newFakeJobStore()
	mkPool(js).handle(Job{ID: "a", Attempts: 1})
	if _, ok := js.retried["a"]; !ok {
		t.Fatal("attempt 1 should be rescheduled")
	}
	if _, ok := js.failed["a"]; ok {
		t.Fatal("attempt 1 should not fail permanently")
	}
	if d := time.Until(js.retried["a"]); d < 20*time.Second || d > 40*time.Second {
		t.Errorf("attempt 1 backoff = %v, want ~30s", d)
	}

	// Attempt 2 of 2 -> reschedule with ~2x backoff.
	js = newFakeJobStore()
	mkPool(js).handle(Job{ID: "a", Attempts: 2})
	if d := time.Until(js.retried["a"]); d < 50*time.Second || d > 70*time.Second {
		t.Errorf("attempt 2 backoff = %v, want ~60s", d)
	}

	// Attempt 3 exceeds MaxRetries -> permanent failure, no reschedule.
	js = newFakeJobStore()
	mkPool(js).handle(Job{ID: "a", Attempts: 3})
	if _, ok := js.failed["a"]; !ok {
		t.Fatal("attempt 3 should fail permanently")
	}
	if _, ok := js.retried["a"]; ok {
		t.Fatal("attempt 3 should not be rescheduled")
	}
}

func TestPoolStartResetsStuckJobsThenProcessesAClaim(t *testing.T) {
	store := localStore(t)
	origKey := "originals/live/original.wav"
	if _, err := store.Put(context.Background(), origKey, strings.NewReader("SOURCE")); err != nil {
		t.Fatalf("seed: %v", err)
	}
	assets := &fakeAssets{original: Original{StorageKey: origKey, Filename: "song.wav"}}
	engine := &fakeEngine{probe: &ffmpeg.ProbeResult{Media: ffmpeg.MediaAudio, DurationSec: 90}}
	notifier := &fakeNotifier{}
	js := newFakeJobStore(Job{
		ID: "live", MediaType: "audio", RecipientEmail: "c@e.com", WatermarkKind: "both", Attempts: 1,
	})

	p := New(testConfig(), js, assets, store, engine, notifier, testutil.Logger())
	p.Start(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	})

	eventually(t, func() bool { return js.doneSet()["live"] })

	if len(js.resetCalls) == 0 {
		t.Error("Start should have called ResetStuck once")
	} else if cutoff := js.resetCalls[0]; time.Since(cutoff) < 55*time.Minute {
		t.Errorf("ResetStuck cutoff = %v, want ~1h ago", cutoff)
	}
	if prev, ok := assets.lastPreview(); !ok || prev.StorageKey != "previews/live/song-preview.m4a" {
		t.Errorf("preview = %+v ok=%v", prev, ok)
	}
	if got, _ := notifier.call(); got != "c@e.com|preview-live" {
		t.Errorf("notifier call = %q", got)
	}
}

func TestPreviewFilename(t *testing.T) {
	cases := []struct{ in, media, want string }{
		{"holiday.MOV", ffmpeg.MediaVideo, "holiday-preview.mp4"},
		{"track.flac", ffmpeg.MediaAudio, "track-preview.m4a"},
		{"photo.jpeg", ffmpeg.MediaImage, "photo-preview.jpeg"},
		{"noext", ffmpeg.MediaImage, "noext-preview.png"},
		{"a/b/c/clip.webm", ffmpeg.MediaVideo, "clip-preview.mp4"},
	}
	for _, c := range cases {
		if got := previewFilename(c.in, c.media); got != c.want {
			t.Errorf("previewFilename(%q, %q) = %q, want %q", c.in, c.media, got, c.want)
		}
	}
}
