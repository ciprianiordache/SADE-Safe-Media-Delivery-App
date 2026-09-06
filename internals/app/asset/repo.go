package asset

import (
	"errors"
	"sort"

	"sade/internals/database"

	crud "github.com/ciprianiordache/crud-depot"
)

// Repo is the persistence boundary for job files. Assets are written by the
// upload path (the original) and the worker (the preview); no client creates
// one directly, so there is no create/update DTO and no HTTP handler - the
// job and share layers compose this Repo the way auth composes magic_token.
type Repo interface {
	Create(a *Asset) (id string, err error)
	GetByID(id string) (*Asset, error)
	// ListByJob returns a job's assets oldest-first (original before preview).
	// An empty result is ([]Asset{}, nil), not an error.
	ListByJob(jobID string) ([]Asset, error)
	// GetByJobAndKind returns the single asset of a kind for a job, or
	// ErrNotFound. The upload path writes one KindOriginal, the worker one
	// KindPreview.
	GetByJobAndKind(jobID, kind string) (*Asset, error)
	Delete(id string) error
}

type repo struct {
	db *database.Database
}

// NewRepo builds the asset repository over the shared connection.
func NewRepo(db *database.Database) Repo { return &repo{db: db} }

func (r *repo) Create(a *Asset) (string, error) {
	return r.db.CRUD().Create(a)
}

func (r *repo) GetByID(id string) (*Asset, error) {
	var a Asset
	err := r.db.CRUD().ReadOne(Asset{}, "id", id, &a)
	if errors.Is(err, crud.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *repo) ListByJob(jobID string) ([]Asset, error) {
	var assets []Asset
	err := r.db.CRUD().Read(Asset{}, "job_id", jobID, &assets)
	if errors.Is(err, crud.ErrNotFound) {
		return []Asset{}, nil // an empty page is not an error
	}
	if err != nil {
		return nil, err
	}
	// Read carries no ORDER BY; present them oldest-first for a stable API.
	sort.Slice(assets, func(i, j int) bool {
		return assets[i].CreatedAt.Before(assets[j].CreatedAt)
	})
	return assets, nil
}

func (r *repo) GetByJobAndKind(jobID, kind string) (*Asset, error) {
	assets, err := r.ListByJob(jobID)
	if err != nil {
		return nil, err
	}
	for i := range assets {
		if assets[i].Kind == kind {
			return &assets[i], nil
		}
	}
	return nil, ErrNotFound
}

func (r *repo) Delete(id string) error {
	err := r.db.CRUD().Delete(Asset{}, "id", id)
	if errors.Is(err, crud.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
