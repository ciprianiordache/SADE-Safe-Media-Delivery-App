// Package login_event records each successful sign-in: which account, how
// (link, code or passkey), from which browser and IP. It backs the "Recent
// activity" list on the account's security page and decides whether a
// sign-in comes from a device the account has not used before (which earns
// the owner an alert email). Internal to the auth service - no DTOs.
package login_event

import "time"

// Sign-in methods.
const (
	MethodLink    = "link"
	MethodCode    = "code"
	MethodPasskey = "passkey"
)

// LoginEvent is one successful sign-in.
type LoginEvent struct {
	ID        string    `db:"id,primary_key,uuid"`
	UserID    string    `db:"user_id,notnull,index,references:users(id),on_delete:cascade"`
	Method    string    `db:"method,notnull"`
	UserAgent string    `db:"user_agent"`
	IP        string    `db:"ip"`
	CreatedAt time.Time `db:"created_at,oncreate,index"`
}

// TableName is the SQL table (used by both schema-builder and crud-depot).
func (LoginEvent) TableName() string { return "login_events" }
