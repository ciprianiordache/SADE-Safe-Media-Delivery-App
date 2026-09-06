// Package worker is the in-process pool that turns pending jobs into
// watermarked previews. The database row is the source of truth for job
// state: the pool claims pending rows (FOR UPDATE SKIP LOCKED on Postgres),
// runs the ffmpeg engine, stores the preview + its asset row, emails the
// recipient a signed link, and drives the status transitions
// pending -> processing -> done | failed with bounded, backed-off retries.
//
// The pool depends only on the small interfaces declared here, so it is wired
// with thin adapters over the job/asset repositories and tested with fakes -
// no ffmpeg, database or SMTP needed.
package worker

import (
	"context"
	"io"
	"time"

	"sade/internals/ffmpeg"
	"sade/internals/storage"
)

// Job is the pool's view of a claimed job row, decoupled from
// internals/app/job so the worker never imports the HTTP domain layer.
type Job struct {
	ID             string
	UserID         string
	MediaType      string
	RecipientEmail string
	WatermarkKind  string // "logo" | "text" | "both" (ffmpeg overlay kinds)
	WatermarkText  string // "" -> render from the configured template
	WatermarkOpts  string // raw JSON overriding ffmpeg defaults; "" -> defaults
	Attempts       int    // times attempted, including the current claim
}

// JobStore claims job rows and records their outcome. Every method maps to an
// atomic statement against the jobs table.
type JobStore interface {
	ClaimPending(ctx context.Context, limit int, now time.Time) ([]Job, error)
	MarkDone(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id, errMsg string) error
	MarkForRetry(ctx context.Context, id, errMsg string, nextAttemptAt time.Time) error
	// ResetStuck requeues rows left in 'processing' since before cutoff (a
	// crashed run) and reports how many were reset.
	ResetStuck(ctx context.Context, cutoff time.Time) (int, error)
}

// Original is what the pool needs about a job's uploaded source file.
type Original struct {
	StorageKey string
	Filename   string
}

// PreviewInput is the metadata for the preview asset row the pool writes when
// a job succeeds.
type PreviewInput struct {
	JobID      string
	StorageKey string
	Filename   string
	MIME       string
	SizeBytes  int64
	Checksum   string // sha256 hex
}

// AssetStore reads a job's original and records its preview.
type AssetStore interface {
	Original(ctx context.Context, jobID string) (Original, error)
	AddPreview(ctx context.Context, p PreviewInput) (previewID string, err error)
}

// Blob is the subset of internals/storage the pool uses. *storage.Storage
// backends satisfy it directly.
type Blob interface {
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Stat(ctx context.Context, key string) (storage.FileInfo, error)
	LocalPath(key string) (path string, ok bool)
}

// Engine is the watermark engine. *ffmpeg.Engine satisfies it directly.
type Engine interface {
	Probe(ctx context.Context, path string) (*ffmpeg.ProbeResult, error)
	Watermark(ctx context.Context, req ffmpeg.Request) error
}

// Notifier delivers the "preview ready" message. previewID is the id of the
// preview asset row, which the notifier turns into a signed share link.
type Notifier interface {
	PreviewReady(ctx context.Context, recipient, previewID string) error
}

// Config is the pool's tunables, lifted from config.WorkerConfig (durations
// already resolved) plus the watermark text template.
type Config struct {
	Concurrency     int
	PollInterval    time.Duration
	ClaimBatchSize  int
	MaxRetries      int
	RetryBackoff    time.Duration // base; doubled per prior attempt
	JobTimeout      time.Duration
	StuckJobTimeout time.Duration
	ShutdownGrace   time.Duration
	TextTemplate    string // e.g. "{recipient} · {date}"
}

func (c *Config) withDefaults() {
	if c.Concurrency < 1 {
		c.Concurrency = 1
	}
	if c.PollInterval <= 0 {
		c.PollInterval = 5 * time.Second
	}
	if c.ClaimBatchSize < 1 {
		c.ClaimBatchSize = c.Concurrency
	}
	if c.MaxRetries < 0 {
		c.MaxRetries = 0
	}
	if c.RetryBackoff <= 0 {
		c.RetryBackoff = 30 * time.Second
	}
	if c.JobTimeout <= 0 {
		c.JobTimeout = 30 * time.Minute
	}
	if c.StuckJobTimeout <= 0 {
		c.StuckJobTimeout = time.Hour
	}
	if c.ShutdownGrace <= 0 {
		c.ShutdownGrace = time.Minute
	}
}
