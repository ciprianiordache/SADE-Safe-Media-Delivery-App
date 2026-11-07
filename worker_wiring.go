package main

import (
	"context"
	"io"
	"log/slog"
	"time"

	"sade/internals/app/asset"
	"sade/internals/app/job"
	"sade/internals/app/job_event"
	"sade/internals/storage"
	"sade/internals/wmopts"
	"sade/internals/worker"
)

// The worker package depends only on its own small interfaces. These adapters
// bind those interfaces to the job/asset repositories: they add the
// context.Context the worker passes through and translate between the domain
// structs and the worker's decoupled DTOs.

type jobStoreAdapter struct{ repo job.Repo }

func (a jobStoreAdapter) ClaimPending(_ context.Context, limit int, now time.Time) ([]worker.Job, error) {
	rows, err := a.repo.ClaimPending(limit, now)
	if err != nil {
		return nil, err
	}
	out := make([]worker.Job, len(rows))
	for i, r := range rows {
		out[i] = worker.Job{
			ID:             r.ID,
			UserID:         r.UserID,
			MediaType:      r.MediaType,
			RecipientEmail: r.RecipientEmail,
			WatermarkKind:  r.WatermarkKind,
			WatermarkText:  r.WatermarkText,
			WatermarkOpts:  r.WatermarkOpts,
			Attempts:       r.Attempts,
		}
	}
	return out, nil
}

func (a jobStoreAdapter) MarkDone(_ context.Context, id string) error { return a.repo.MarkDone(id) }

func (a jobStoreAdapter) MarkFailed(_ context.Context, id, errMsg string) error {
	return a.repo.MarkFailed(id, errMsg)
}

func (a jobStoreAdapter) MarkForRetry(_ context.Context, id, errMsg string, nextAttemptAt time.Time) error {
	return a.repo.MarkForRetry(id, errMsg, nextAttemptAt)
}

func (a jobStoreAdapter) ResetStuck(_ context.Context, cutoff time.Time) (int, error) {
	return a.repo.ResetStuck(cutoff)
}

type assetStoreAdapter struct{ repo asset.Repo }

func (a assetStoreAdapter) Original(_ context.Context, jobID string) (worker.Original, error) {
	row, err := a.repo.GetByJobAndKind(jobID, asset.KindOriginal)
	if err != nil {
		return worker.Original{}, err
	}
	return worker.Original{StorageKey: row.StorageKey, Filename: row.Filename}, nil
}

func (a assetStoreAdapter) AddPreview(_ context.Context, p worker.PreviewInput) (string, error) {
	return a.repo.UpsertPreview(&asset.Asset{
		JobID:      p.JobID,
		Kind:       asset.KindPreview,
		StorageKey: p.StorageKey,
		Filename:   p.Filename,
		MIME:       p.MIME,
		SizeBytes:  p.SizeBytes,
		Checksum:   p.Checksum,
	})
}

// recordingNotifier sends the preview-ready email and, once it went out,
// adds "sent" to the job's delivery history.
type recordingNotifier struct {
	inner  worker.Notifier
	assets asset.Repo
	events job_event.Repo
	log    *slog.Logger
}

func (n recordingNotifier) PreviewReady(ctx context.Context, recipient, previewID string) error {
	if err := n.inner.PreviewReady(ctx, recipient, previewID); err != nil {
		return err
	}
	a, err := n.assets.GetByID(previewID)
	if err != nil {
		n.log.Warn("delivery: find job of preview", "preview", previewID, "error", err)
		return nil
	}
	if err := n.events.Record(a.JobID, job_event.KindSent, recipient); err != nil {
		n.log.Warn("delivery: record sent", "job", a.JobID, "error", err)
	}
	return nil
}

// emailBranding finds the brand frozen on a preview's job, for the
// preview-ready email: the company name and its logo bytes (best effort -
// any miss just means an unbranded email).
func emailBranding(assets asset.Repo, jobs job.Repo, store storage.Storage) func(context.Context, string) (string, []byte) {
	return func(ctx context.Context, previewID string) (string, []byte) {
		a, err := assets.GetByID(previewID)
		if err != nil {
			return "", nil
		}
		j, err := jobs.GetByID(a.JobID)
		if err != nil {
			return "", nil
		}
		b := wmopts.Decode(j.WatermarkOpts)
		if b.LogoKey == "" {
			return b.Company, nil
		}
		rc, err := store.Open(ctx, b.LogoKey)
		if err != nil {
			return b.Company, nil
		}
		defer rc.Close()
		logo, err := io.ReadAll(io.LimitReader(rc, 2<<20))
		if err != nil {
			return b.Company, nil
		}
		return b.Company, logo
	}
}
