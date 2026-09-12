package payment

import (
	"errors"
	"sort"

	"sade/internals/database"

	crud "github.com/ciprianiordache/crud-depot"
)

// Repo is the persistence boundary for payments.
type Repo interface {
	Create(p *Payment) (id string, err error)
	// LatestByJob returns the most recently created payment for a job (an
	// operator's own retried checkouts all point at the same job), or
	// ErrNotFound if the job has none yet.
	LatestByJob(jobID string) (*Payment, error)
	// ByProviderRef looks a payment up by its Stripe Checkout Session id -
	// the only handle the webhook payload carries.
	ByProviderRef(ref string) (*Payment, error)
	Update(p *Payment) error
}

type repo struct{ db *database.Database }

func NewRepo(db *database.Database) Repo { return &repo{db: db} }

func (r *repo) Create(p *Payment) (string, error) {
	return r.db.CRUD().Create(p)
}

func (r *repo) LatestByJob(jobID string) (*Payment, error) {
	var payments []Payment
	err := r.db.CRUD().Read(Payment{}, "job_id", jobID, &payments)
	if errors.Is(err, crud.ErrNotFound) || len(payments) == 0 {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	// Read has no ORDER BY: sort newest-first in memory (payment volume per
	// job is tiny - at most a handful of checkout attempts).
	sort.Slice(payments, func(i, j int) bool {
		return payments[i].CreatedAt.After(payments[j].CreatedAt)
	})
	return &payments[0], nil
}

func (r *repo) ByProviderRef(ref string) (*Payment, error) {
	var payments []Payment
	err := r.db.CRUD().Read(Payment{}, "provider_ref", ref, &payments)
	if errors.Is(err, crud.ErrNotFound) || len(payments) == 0 {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &payments[0], nil
}

func (r *repo) Update(p *Payment) error {
	err := r.db.CRUD().Update(p, "id", p.ID)
	if errors.Is(err, crud.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
