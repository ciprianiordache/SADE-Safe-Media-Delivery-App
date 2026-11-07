package brand

import (
	"errors"
	"fmt"

	"sade/internals/database"

	crud "github.com/ciprianiordache/crud-depot"
)

// ErrNotFound: the account has no brand row yet.
var ErrNotFound = errors.New("brand: not found")

// Repo persists brand settings.
type Repo interface {
	GetByUser(userID string) (*Brand, error)
	// Save inserts b (no ID yet) or updates it.
	Save(b *Brand) error
}

type repo struct {
	db *database.Database
}

func NewRepo(db *database.Database) Repo { return &repo{db: db} }

func (r *repo) GetByUser(userID string) (*Brand, error) {
	var b Brand
	err := r.db.CRUD().ReadOne(Brand{}, "user_id", userID, &b)
	if errors.Is(err, crud.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("brand: get: %w", err)
	}
	return &b, nil
}

func (r *repo) Save(b *Brand) error {
	if b.ID == "" {
		id, err := r.db.CRUD().Create(b)
		if err != nil {
			return fmt.Errorf("brand: create: %w", err)
		}
		b.ID = id
		return nil
	}
	if err := r.db.CRUD().Update(b, "id", b.ID); err != nil {
		return fmt.Errorf("brand: update: %w", err)
	}
	return nil
}
