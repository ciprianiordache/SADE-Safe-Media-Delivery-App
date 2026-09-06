package job

import (
	"errors"
	"sort"

	"sade/internals/database"

	crud "github.com/ciprianiordache/crud-depot"
)

// Repo is the persistence boundary for jobs. The database row is the source of
// truth for job state; the worker owns the status transitions via Update.
type Repo interface {
	Create(j *Job) (id string, err error)
	GetByID(id string) (*Job, error)
	// ListByUser returns one operator's jobs newest-first, windowed by
	// offset/limit. An empty page is ([]Job{}, nil).
	ListByUser(userID string, offset, limit int) ([]Job, error)
	Update(j *Job) error
	Delete(id string) error
}

type repo struct {
	db *database.Database
}

// NewRepo builds the job repository over the shared connection.
func NewRepo(db *database.Database) Repo { return &repo{db: db} }

func (r *repo) Create(j *Job) (string, error) {
	return r.db.CRUD().Create(j)
}

func (r *repo) GetByID(id string) (*Job, error) {
	var j Job
	err := r.db.CRUD().ReadOne(Job{}, "id", id, &j)
	if errors.Is(err, crud.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func (r *repo) ListByUser(userID string, offset, limit int) ([]Job, error) {
	var jobs []Job
	err := r.db.CRUD().Read(Job{}, "user_id", userID, &jobs)
	if errors.Is(err, crud.ErrNotFound) {
		return []Job{}, nil
	}
	if err != nil {
		return nil, err
	}
	// Read has no ORDER BY / LIMIT: sort newest-first and window in memory.
	// Job volume per operator is small; a SQL-side page can come later.
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.After(jobs[j].CreatedAt)
	})
	if offset >= len(jobs) {
		return []Job{}, nil
	}
	end := offset + limit
	if end > len(jobs) {
		end = len(jobs)
	}
	return jobs[offset:end], nil
}

func (r *repo) Update(j *Job) error {
	err := r.db.CRUD().Update(j, "id", j.ID)
	if errors.Is(err, crud.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func (r *repo) Delete(id string) error {
	err := r.db.CRUD().Delete(Job{}, "id", id)
	if errors.Is(err, crud.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
