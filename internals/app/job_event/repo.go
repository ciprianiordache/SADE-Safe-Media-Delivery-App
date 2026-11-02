package job_event

import (
	"fmt"
	"time"

	"sade/internals/database"
)

// Repo persists delivery history.
type Repo interface {
	// Record stores an event now.
	Record(jobID, kind, detail string) error
	// RecordOnce stores an event unless one of the same kind was stored for
	// the job within window - a video player fires many range requests per
	// view, and that should read as one "opened".
	RecordOnce(jobID, kind string, window time.Duration) error
	// ListByJob returns the job's events, newest first.
	ListByJob(jobID string) ([]JobEvent, error)
	// CountSince counts kind events of the job at or after since.
	CountSince(jobID, kind string, since time.Time) (int, error)
}

type repo struct {
	db  *database.Database
	now func() time.Time
}

func NewRepo(db *database.Database) Repo {
	return &repo{db: db, now: func() time.Time { return time.Now().UTC() }}
}

func (r *repo) Record(jobID, kind, detail string) error {
	if _, err := r.db.CRUD().Create(&JobEvent{JobID: jobID, Kind: kind, Detail: detail, CreatedAt: r.now()}); err != nil {
		return fmt.Errorf("job_event: record %s: %w", kind, err)
	}
	return nil
}

func (r *repo) RecordOnce(jobID, kind string, window time.Duration) error {
	n, err := r.CountSince(jobID, kind, r.now().Add(-window))
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	return r.Record(jobID, kind, "")
}

func (r *repo) ListByJob(jobID string) ([]JobEvent, error) {
	rows, err := r.db.Query(r.db.Rebind(
		`SELECT id, job_id, kind, detail, created_at FROM job_events
		 WHERE job_id = ? ORDER BY created_at DESC LIMIT 100`), jobID)
	if err != nil {
		return nil, fmt.Errorf("job_event: list: %w", err)
	}
	defer rows.Close()
	out := []JobEvent{}
	for rows.Next() {
		var e JobEvent
		if err := rows.Scan(&e.ID, &e.JobID, &e.Kind, &e.Detail, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("job_event: scan: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *repo) CountSince(jobID, kind string, since time.Time) (int, error) {
	var n int
	err := r.db.QueryRow(r.db.Rebind(
		`SELECT COUNT(*) FROM job_events WHERE job_id = ? AND kind = ? AND created_at >= ?`),
		jobID, kind, since).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("job_event: count: %w", err)
	}
	return n, nil
}
