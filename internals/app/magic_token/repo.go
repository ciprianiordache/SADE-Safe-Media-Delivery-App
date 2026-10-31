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
	// Peek reports which user a still-valid token belongs to without
	// spending it (ErrNotFound / ErrExpired otherwise). It backs the
	// sign-in confirmation page, which shows the account before the user
	// commits - so a link opened by a mail scanner, or planted by someone
	// else, never signs anyone in on its own.
	Peek(hash string, now time.Time) (userID string, err error)
	// DeleteExpired removes every token past its expiry; returns the count.
	DeleteExpired(now time.Time) (int, error)
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

func (r *repo) Peek(hash string, now time.Time) (string, error) {
	var mt MagicToken
	if err := r.db.CRUD().ReadOne(MagicToken{}, "token_hash", hash, &mt); err != nil {
		if errors.Is(err, crud.ErrNotFound) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("magic_token: peek: %w", err)
	}
	if mt.Expired(now) {
		return "", ErrExpired
	}
	return mt.UserID, nil
}

func (r *repo) DeleteExpired(now time.Time) (int, error) {
	res, err := r.db.Exec(r.db.Rebind("DELETE FROM magic_tokens WHERE expires_at < ?"), now)
	if err != nil {
		return 0, fmt.Errorf("magic_token: delete expired: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
