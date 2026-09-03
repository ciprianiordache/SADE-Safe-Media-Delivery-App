package magic_token

import (
	"errors"
	"fmt"
	"time"

	"sade/internals/database"

	crud "github.com/ciprianiordache/crud-depot"
)

// Repo persists magic-link tokens.
type Repo interface {
	Create(t *MagicToken) (id string, err error)
	// Consume atomically validates and removes the token with the given
	// hash, returning the user it belongs to. It returns ErrExpired if the
	// token exists but has lapsed, and ErrNotFound if it does not exist or
	// was already consumed (including losing a race to a concurrent call).
	Consume(hash string, now time.Time) (userID string, err error)
}

type repo struct {
	db *database.Database
}

func NewRepo(db *database.Database) Repo { return &repo{db: db} }

func (r *repo) Create(t *MagicToken) (string, error) {
	return r.db.CRUD().Create(t)
}

func (r *repo) Consume(hash string, now time.Time) (string, error) {
	var userID string
	err := r.db.CRUD().RunInTx(r.db, func(tx *crud.CRUD) error {
		var mt MagicToken
		if e := tx.ReadOne(MagicToken{}, "token_hash", hash, &mt); e != nil {
			if errors.Is(e, crud.ErrNotFound) {
				return ErrNotFound
			}
			return e
		}
		if now.After(mt.ExpiresAt) {
			return ErrExpired
		}
		// The RowsAffected check inside crud.Delete makes this the atomic
		// single-use gate: a second caller deleting the same row sees 0
		// rows and gets ErrNotFound.
		if e := tx.Delete(MagicToken{}, "token_hash", hash); e != nil {
			if errors.Is(e, crud.ErrNotFound) {
				return ErrNotFound
			}
			return e
		}
		userID = mt.UserID
		return nil
	})
	switch {
	case err == nil:
		return userID, nil
	case errors.Is(err, ErrNotFound):
		return "", ErrNotFound
	case errors.Is(err, ErrExpired):
		return "", ErrExpired
	default:
		return "", fmt.Errorf("magic_token: consume: %w", err)
	}
}
