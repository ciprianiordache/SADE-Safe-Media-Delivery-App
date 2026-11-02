// Package passkey stores an account's WebAuthn credentials (passkeys:
// fingerprint, face, Windows Hello, a security key). The verification itself
// - challenges, signatures - lives in the auth service via go-webauthn; this
// package only keeps the credential records. Internal - no DTOs here; the
// auth handler renders its own view.
package passkey

import "time"

// Passkey is one registered credential.
type Passkey struct {
	ID     string `db:"id,primary_key,uuid"`
	UserID string `db:"user_id,notnull,index,references:users(id),on_delete:cascade"`
	// CredentialID is the authenticator's credential id, base64url - what a
	// sign-in assertion names, so it is the lookup key.
	CredentialID string `db:"credential_id,notnull,unique"`
	// Credential is go-webauthn's webauthn.Credential as JSON (public key,
	// sign counter, flags, transports). Stored whole so a library upgrade
	// that adds fields needs no schema change.
	Credential string    `db:"credential,notnull"`
	Name       string    `db:"name"`
	CreatedAt  time.Time `db:"created_at,oncreate"`
	LastUsedAt time.Time `db:"last_used_at"`
}

// TableName is the SQL table (used by both schema-builder and crud-depot).
func (Passkey) TableName() string { return "passkeys" }
