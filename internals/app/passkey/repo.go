package passkey

import (
	"errors"
	"fmt"
	"time"

	"sade/internals/database"

	crud "github.com/ciprianiordache/crud-depot"
)

// ErrNotFound is returned for an unknown passkey (or one owned by another
// account).
var ErrNotFound = errors.New("passkey: not found")

// Repo persists passkeys.
type Repo interface {
	Create(p *Passkey) (id string, err error)
	ListByUser(userID string) ([]Passkey, error)
	GetByCredentialID(credentialID string) (*Passkey, error)
	// Used stores the credential after a sign-in (its sign counter moves)
	// and stamps LastUsedAt.
	Used(id, credentialJSON string, at time.Time) error
	// Delete removes userID's passkey id; ErrNotFound if it isn't theirs.
	Delete(userID, id string) error
}

type repo struct {
	db *database.Database
}

func NewRepo(db *database.Database) Repo { return &repo{db: db} }

func (r *repo) Create(p *Passkey) (string, error) {
	return r.db.CRUD().Create(p)
}

func (r *repo) ListByUser(userID string) ([]Passkey, error) {
	var out []Passkey
	err := r.db.CRUD().Read(Passkey{}, "user_id", userID, &out)
	if errors.Is(err, crud.ErrNotFound) {
		return []Passkey{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("passkey: list: %w", err)
	}
	return out, nil
}

func (r *repo) GetByCredentialID(credentialID string) (*Passkey, error) {
	var p Passkey
	err := r.db.CRUD().ReadOne(Passkey{}, "credential_id", credentialID, &p)
	if errors.Is(err, crud.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("passkey: get: %w", err)
	}
	return &p, nil
}

func (r *repo) Used(id, credentialJSON string, at time.Time) error {
	_, err := r.db.Exec(r.db.Rebind(`UPDATE passkeys SET credential = ?, last_used_at = ? WHERE id = ?`),
		credentialJSON, at, id)
	if err != nil {
		return fmt.Errorf("passkey: update: %w", err)
	}
	return nil
}

func (r *repo) Delete(userID, id string) error {
	res, err := r.db.Exec(r.db.Rebind(`DELETE FROM passkeys WHERE id = ? AND user_id = ?`), id, userID)
	if err != nil {
		return fmt.Errorf("passkey: delete: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
