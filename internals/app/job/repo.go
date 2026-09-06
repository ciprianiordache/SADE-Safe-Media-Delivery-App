package job

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"sade/internals/database"

	crud "github.com/ciprianiordache/crud-depot"
)

// maxErrLen bounds the failure message stored on a job row.
const maxErrLen = 1000

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

	// --- worker-facing: the job row is the source of truth for state ---

	// ClaimPending atomically moves up to limit eligible pending jobs
	// (status='pending' and next_attempt_at <= now) to status='processing',
	// bumps their attempts, and returns them oldest-first. On Postgres the
	// claim uses FOR UPDATE SKIP LOCKED so parallel workers never collide.
	ClaimPending(limit int, now time.Time) ([]Job, error)
	// MarkDone finishes a job (status='done'); it is a no-op guard against a
	// row that is no longer 'processing'.
	MarkDone(id string) error
	// MarkFailed ends a job terminally (status='failed') with errMsg.
	MarkFailed(id, errMsg string) error
	// MarkForRetry returns a job to 'pending' with errMsg recorded and
	// next_attempt_at set to nextAttemptAt (backoff).
	MarkForRetry(id, errMsg string, nextAttemptAt time.Time) error
	// ResetStuck returns jobs left in 'processing' since before cutoff (a
	// crashed worker) to 'pending', ready immediately. Returns the row count.
	ResetStuck(cutoff time.Time) (int, error)
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

// claimColumns is the projection ClaimPending scans, in order.
const claimColumns = "id, user_id, status, media_type, recipient_email, " +
	"watermark_kind, watermark_text, watermark_opts, attempts"

func (r *repo) ClaimPending(limit int, now time.Time) ([]Job, error) {
	if limit < 1 {
		limit = 1
	}
	// Select the eligible ids, lock-skipping on Postgres, then flip them in
	// one UPDATE and return the claimed rows.
	inner := "SELECT id FROM jobs WHERE status = 'pending' AND next_attempt_at <= ? " +
		"ORDER BY created_at LIMIT ?"
	if r.isPostgres() {
		inner += " FOR UPDATE SKIP LOCKED"
	}
	q := r.rebind(fmt.Sprintf(
		"UPDATE jobs SET status = 'processing', attempts = attempts + 1, updated_at = ? "+
			"WHERE id IN (%s) RETURNING %s",
		inner, claimColumns,
	))

	rows, err := r.db.Query(q, now, now, limit)
	if err != nil {
		return nil, fmt.Errorf("job: claim pending: %w", err)
	}
	defer rows.Close()

	var claimed []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(
			&j.ID, &j.UserID, &j.Status, &j.MediaType, &j.RecipientEmail,
			&j.WatermarkKind, &j.WatermarkText, &j.WatermarkOpts, &j.Attempts,
		); err != nil {
			return nil, fmt.Errorf("job: scan claimed row: %w", err)
		}
		claimed = append(claimed, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("job: claim pending rows: %w", err)
	}
	return claimed, nil
}

func (r *repo) MarkDone(id string) error {
	_, err := r.db.Exec(
		r.rebind("UPDATE jobs SET status = 'done', updated_at = ? WHERE id = ? AND status = 'processing'"),
		time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("job: mark done: %w", err)
	}
	return nil
}

func (r *repo) MarkFailed(id, errMsg string) error {
	_, err := r.db.Exec(
		r.rebind("UPDATE jobs SET status = 'failed', error = ?, updated_at = ? WHERE id = ?"),
		truncate(errMsg, maxErrLen), time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("job: mark failed: %w", err)
	}
	return nil
}

func (r *repo) MarkForRetry(id, errMsg string, nextAttemptAt time.Time) error {
	_, err := r.db.Exec(
		r.rebind("UPDATE jobs SET status = 'pending', error = ?, next_attempt_at = ?, updated_at = ? WHERE id = ?"),
		truncate(errMsg, maxErrLen), nextAttemptAt, time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("job: mark for retry: %w", err)
	}
	return nil
}

func (r *repo) ResetStuck(cutoff time.Time) (int, error) {
	res, err := r.db.Exec(
		r.rebind("UPDATE jobs SET status = 'pending', next_attempt_at = ?, updated_at = ? "+
			"WHERE status = 'processing' AND updated_at < ?"),
		time.Time{}, time.Now(), cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("job: reset stuck: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (r *repo) isPostgres() bool {
	switch r.db.Driver() {
	case "pgx", "postgres":
		return true
	default:
		return false
	}
}

// rebind turns the '?' placeholders in q into $1, $2, ... for Postgres and
// leaves them untouched for SQLite.
func (r *repo) rebind(q string) string {
	if !r.isPostgres() {
		return q
	}
	var b strings.Builder
	n := 0
	for _, c := range q {
		if c == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteRune(c)
	}
	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
