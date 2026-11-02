package login_event

import (
	"fmt"
	"time"

	"sade/internals/database"
)

// Repo persists sign-in history.
type Repo interface {
	Create(e *LoginEvent) error
	// Recent returns userID's newest events, newest first.
	Recent(userID string, limit int) ([]LoginEvent, error)
	// SeenUserAgent reports whether userID signed in with this exact user
	// agent at or after since.
	SeenUserAgent(userID, userAgent string, since time.Time) (bool, error)
	// DeleteBefore drops events older than cutoff; returns the count.
	DeleteBefore(cutoff time.Time) (int, error)
}

type repo struct {
	db *database.Database
}

func NewRepo(db *database.Database) Repo { return &repo{db: db} }

func (r *repo) Create(e *LoginEvent) error {
	if _, err := r.db.CRUD().Create(e); err != nil {
		return fmt.Errorf("login_event: create: %w", err)
	}
	return nil
}

func (r *repo) Recent(userID string, limit int) ([]LoginEvent, error) {
	rows, err := r.db.Query(r.db.Rebind(
		`SELECT id, user_id, method, user_agent, ip, created_at FROM login_events
		 WHERE user_id = ? ORDER BY created_at DESC LIMIT ?`), userID, limit)
	if err != nil {
		return nil, fmt.Errorf("login_event: recent: %w", err)
	}
	defer rows.Close()
	out := []LoginEvent{}
	for rows.Next() {
		var e LoginEvent
		if err := rows.Scan(&e.ID, &e.UserID, &e.Method, &e.UserAgent, &e.IP, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("login_event: scan: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *repo) SeenUserAgent(userID, userAgent string, since time.Time) (bool, error) {
	var n int
	err := r.db.QueryRow(r.db.Rebind(
		`SELECT COUNT(*) FROM login_events WHERE user_id = ? AND user_agent = ? AND created_at >= ?`),
		userID, userAgent, since).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("login_event: seen: %w", err)
	}
	return n > 0, nil
}

func (r *repo) DeleteBefore(cutoff time.Time) (int, error) {
	res, err := r.db.Exec(r.db.Rebind(`DELETE FROM login_events WHERE created_at < ?`), cutoff)
	if err != nil {
		return 0, fmt.Errorf("login_event: delete old: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
