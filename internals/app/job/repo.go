package job

import (
	"database/sql"
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

	// CountCreatedSince counts userID's jobs created at or after since.
	CountCreatedSince(userID string, since time.Time) (int, error)
	// StorageBytes sums the size of every asset across userID's jobs.
	StorageBytes(userID string) (int64, error)
	// HasPaidPayment reports whether any payment for jobID is paid.
	HasPaidPayment(jobID string) (bool, error)
	// DeliverySummary reports, for each of jobIDs, when the recipient last
	// opened the preview and whether the job was paid for. Jobs with
	// neither are absent from the map. Two queries for the whole page.
	DeliverySummary(jobIDs []string) (map[string]DeliverySummary, error)
	// SetLinksDisabled switches the job's public links off (true) or on.
	SetLinksDisabled(jobID string, disabled bool) error
	// LinksDisabled reports whether the job's public links are switched
	// off. An unknown job reads as ErrNotFound.
	LinksDisabled(jobID string) (bool, error)
	// DeleteIfNotProcessing removes the job row (cascading its assets and
	// payments) unless the worker holds it; false means it was processing.
	DeleteIfNotProcessing(jobID string) (bool, error)

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

func (r *repo) CountCreatedSince(userID string, since time.Time) (int, error) {
	var n int
	err := r.db.QueryRow(
		r.rebind("SELECT COUNT(*) FROM jobs WHERE user_id = ? AND created_at >= ?"),
		userID, since,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("job: count recent: %w", err)
	}
	return n, nil
}

func (r *repo) StorageBytes(userID string) (int64, error) {
	var n int64
	err := r.db.QueryRow(
		r.rebind("SELECT COALESCE(SUM(a.size_bytes), 0) FROM assets a "+
			"JOIN jobs j ON a.job_id = j.id WHERE j.user_id = ?"),
		userID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("job: storage bytes: %w", err)
	}
	return n, nil
}

func (r *repo) HasPaidPayment(jobID string) (bool, error) {
	var n int
	err := r.db.QueryRow(
		r.rebind("SELECT COUNT(*) FROM payments WHERE job_id = ? AND status = 'paid'"), jobID,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("job: check payments: %w", err)
	}
	return n > 0, nil
}

func (r *repo) DeliverySummary(jobIDs []string) (map[string]DeliverySummary, error) {
	out := map[string]DeliverySummary{}
	if len(jobIDs) == 0 {
		return out, nil
	}
	in := strings.TrimSuffix(strings.Repeat("?,", len(jobIDs)), ",")
	args := make([]any, len(jobIDs))
	for i, id := range jobIDs {
		args[i] = id
	}

	// Raw SQL against job_events/payments: those packages' tests import this
	// one, so importing them here would be a cycle (see HasPaidPayment).
	rows, err := r.db.Query(r.rebind(
		"SELECT job_id, created_at FROM job_events WHERE kind = 'viewed' AND job_id IN ("+in+")"), args...)
	if err != nil {
		return nil, fmt.Errorf("job: delivery opened: %w", err)
	}
	for rows.Next() {
		var id string
		var at time.Time
		if err := rows.Scan(&id, &at); err != nil {
			rows.Close()
			return nil, fmt.Errorf("job: scan opened: %w", err)
		}
		s := out[id]
		if at.After(s.OpenedAt) {
			s.OpenedAt = at
		}
		out[id] = s
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = r.db.Query(r.rebind(
		"SELECT DISTINCT job_id FROM payments WHERE status = 'paid' AND job_id IN ("+in+")"), args...)
	if err != nil {
		return nil, fmt.Errorf("job: delivery paid: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("job: scan paid: %w", err)
		}
		s := out[id]
		s.Paid = true
		out[id] = s
	}
	return out, rows.Err()
}

func (r *repo) SetLinksDisabled(jobID string, disabled bool) error {
	v := 0
	if disabled {
		v = 1
	}
	res, err := r.db.Exec(r.rebind("UPDATE jobs SET links_disabled = ? WHERE id = ?"), v, jobID)
	if err != nil {
		return fmt.Errorf("job: set links: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *repo) LinksDisabled(jobID string) (bool, error) {
	var v int
	err := r.db.QueryRow(r.rebind("SELECT links_disabled FROM jobs WHERE id = ?"), jobID).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("job: read links flag: %w", err)
	}
	return v == 1, nil
}

func (r *repo) DeleteIfNotProcessing(jobID string) (bool, error) {
	// One conditional statement, so the worker cannot claim the row between
	// a status check and the delete.
	res, err := r.db.Exec(
		r.rebind("DELETE FROM jobs WHERE id = ? AND status <> 'processing'"), jobID,
	)
	if err != nil {
		return false, fmt.Errorf("job: delete: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
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
