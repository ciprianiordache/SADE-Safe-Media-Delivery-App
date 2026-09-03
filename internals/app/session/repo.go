package session

import (
	"errors"

	"sade/internals/database"

	crud "github.com/ciprianiordache/crud-depot"
)

// Repo persists login sessions.
type Repo interface {
	Create(s *Session) (id string, err error)
	// GetByHash returns the session for a cookie-value hash, or ErrNotFound.
	GetByHash(hash string) (*Session, error)
	// Delete removes the session with the given hash. Deleting an unknown
	// hash is not an error (logout is idempotent).
	Delete(hash string) error
}

type repo struct {
	db *database.Database
}

func NewRepo(db *database.Database) Repo { return &repo{db: db} }

func (r *repo) Create(s *Session) (string, error) {
	return r.db.CRUD().Create(s)
}

func (r *repo) GetByHash(hash string) (*Session, error) {
	var s Session
	err := r.db.CRUD().ReadOne(Session{}, "token_hash", hash, &s)
	if errors.Is(err, crud.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *repo) Delete(hash string) error {
	err := r.db.CRUD().Delete(Session{}, "token_hash", hash)
	if err == nil || errors.Is(err, crud.ErrNotFound) {
		return nil
	}
	return err
}
