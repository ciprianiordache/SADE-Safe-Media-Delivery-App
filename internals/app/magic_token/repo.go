package magic_token

import (
	"crypto/subtle"
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
	// token exists but has lapsed, ErrOtherBrowser (token kept) if
	// browserHash does not match the requesting browser's, and ErrNotFound
	// if it does not exist or was already consumed (including losing a race
	// to a concurrent call).
	Consume(hash, browserHash string, now time.Time) (userID string, err error)
	// ConsumeByCode signs in with the 6-digit code instead of the link. It
	// looks at userID's still-valid tokens requested by browserHash; if
	// match accepts one of them, all of them are removed and nil returned.
	// Otherwise every one of them counts a failed attempt, tokens reaching
	// maxAttempts are removed, and ErrBadCode is returned (ErrNotFound when
	// there was no candidate token at all).
	ConsumeByCode(userID, browserHash string, match func(MagicToken) bool, maxAttempts int, now time.Time) error
	// Peek reports which user a still-valid token belongs to without
	// spending it (ErrNotFound / ErrExpired otherwise). It backs the
	// sign-in confirmation page, which shows the account before the user
	// commits - so a link opened by a mail scanner, or planted by someone
	// else, never signs anyone in on its own.
	Peek(hash string, now time.Time) (*MagicToken, error)
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

func (r *repo) Consume(hash, browserHash string, now time.Time) (string, error) {
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
		if mt.BrowserHash == "" || subtle.ConstantTimeCompare([]byte(mt.BrowserHash), []byte(browserHash)) != 1 {
			return ErrOtherBrowser
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
	case errors.Is(err, ErrOtherBrowser):
		return "", ErrOtherBrowser
	default:
		return "", fmt.Errorf("magic_token: consume: %w", err)
	}
}

func (r *repo) Peek(hash string, now time.Time) (*MagicToken, error) {
	var mt MagicToken
	if err := r.db.CRUD().ReadOne(MagicToken{}, "token_hash", hash, &mt); err != nil {
		if errors.Is(err, crud.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("magic_token: peek: %w", err)
	}
	if mt.Expired(now) {
		return nil, ErrExpired
	}
	return &mt, nil
}

func (r *repo) ConsumeByCode(userID, browserHash string, match func(MagicToken) bool, maxAttempts int, now time.Time) error {
	if browserHash == "" {
		return ErrNotFound
	}
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("magic_token: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.Query(r.db.Rebind(
		`SELECT id, token_hash, code_hash, attempts FROM magic_tokens
		 WHERE user_id = ? AND browser_hash = ? AND expires_at > ?`), userID, browserHash, now)
	if err != nil {
		return fmt.Errorf("magic_token: candidates: %w", err)
	}
	var cands []MagicToken
	for rows.Next() {
		t := MagicToken{UserID: userID, BrowserHash: browserHash}
		if err := rows.Scan(&t.ID, &t.TokenHash, &t.CodeHash, &t.Attempts); err != nil {
			rows.Close()
			return fmt.Errorf("magic_token: scan: %w", err)
		}
		cands = append(cands, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("magic_token: candidates: %w", err)
	}
	if len(cands) == 0 {
		return ErrNotFound
	}

	matched := false
	for _, t := range cands {
		if t.CodeHash != "" && match(t) {
			matched = true
			break
		}
	}
	if matched {
		// Spend every pending link of this browser, not just the matching
		// one - the user is in, older emails should be dead.
		res, err := tx.Exec(r.db.Rebind(`DELETE FROM magic_tokens WHERE user_id = ? AND browser_hash = ?`), userID, browserHash)
		if err != nil {
			return fmt.Errorf("magic_token: spend: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound // lost a race with a concurrent sign-in
		}
		return tx.Commit()
	}

	for _, t := range cands {
		if t.Attempts+1 >= maxAttempts {
			_, err = tx.Exec(r.db.Rebind(`DELETE FROM magic_tokens WHERE id = ?`), t.ID)
		} else {
			_, err = tx.Exec(r.db.Rebind(`UPDATE magic_tokens SET attempts = attempts + 1 WHERE id = ?`), t.ID)
		}
		if err != nil {
			return fmt.Errorf("magic_token: count attempt: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("magic_token: commit: %w", err)
	}
	return ErrBadCode
}

func (r *repo) DeleteExpired(now time.Time) (int, error) {
	res, err := r.db.Exec(r.db.Rebind("DELETE FROM magic_tokens WHERE expires_at < ?"), now)
	if err != nil {
		return 0, fmt.Errorf("magic_token: delete expired: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
