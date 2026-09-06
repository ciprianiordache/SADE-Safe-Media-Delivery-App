package main

import (
	"context"
	"time"

	"sade/internals/app/asset"
	"sade/internals/app/job"
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
	return a.repo.Create(&asset.Asset{
		JobID:      p.JobID,
		Kind:       asset.KindPreview,
		StorageKey: p.StorageKey,
		Filename:   p.Filename,
		MIME:       p.MIME,
		SizeBytes:  p.SizeBytes,
		Checksum:   p.Checksum,
	})
}
