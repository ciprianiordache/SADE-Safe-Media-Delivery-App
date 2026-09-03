// Package session holds server-side login sessions. The session's secret
// lives in an HttpOnly cookie on the client; only its SHA-256 hash is stored,
// so a database leak does not hand out live sessions. Sessions are created on
// magic-link callback and revoked on logout - no request/response DTOs.
package session

import "time"

// Session is one authenticated login.
type Session struct {
	ID        string    `db:"id,primary_key,uuid"`
	UserID    string    `db:"user_id,notnull,index,references:users(id),on_delete:cascade"`
	TokenHash string    `db:"token_hash,notnull,unique"` // sha256 hex of the cookie value
	ExpiresAt time.Time `db:"expires_at,notnull"`
	CreatedAt time.Time `db:"created_at,oncreate"`
}

// TableName is the SQL table (used by both schema-builder and crud-depot).
func (Session) TableName() string { return "sessions" }

// Expired reports whether the session is past its expiry at the given instant.
func (s Session) Expired(now time.Time) bool { return now.After(s.ExpiresAt) }
