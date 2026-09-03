// Package magic_token holds single-use, short-lived login tokens. The raw
// token is emailed to the user as part of the callback URL; only its SHA-256
// hash is stored here. Single use is enforced by deleting the row on
// consumption (see Repo.Consume) rather than a "used" flag, so two concurrent
// callbacks for the same link cannot both succeed. These records are created
// and consumed entirely by the auth service, so there are no request/response
// DTOs.
package magic_token

import "time"

// MagicToken is one issued login link.
type MagicToken struct {
	ID        string    `db:"id,primary_key,uuid"`
	UserID    string    `db:"user_id,notnull,index,references:users(id),on_delete:cascade"`
	TokenHash string    `db:"token_hash,notnull,unique"` // sha256 hex of the raw token
	ExpiresAt time.Time `db:"expires_at,notnull"`
	CreatedAt time.Time `db:"created_at,oncreate"`
}

// TableName is the SQL table (used by both schema-builder and crud-depot).
func (MagicToken) TableName() string { return "magic_tokens" }

// Expired reports whether the token is past its expiry at the given instant.
func (t MagicToken) Expired(now time.Time) bool { return now.After(t.ExpiresAt) }
