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
	// LogoInUse reports whether an unfinished (pending or processing) job's
	// brand snapshot still points at this logo key - the worker reads the
	// file while it encodes, so such a logo must not be deleted yet.
	LogoInUse(key string) (bool, error)
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

// LogoInUse is raw SQL over jobs (statuses as literals): importing the job
// package here would make job's tests, which build brands, an import cycle.
// The snapshot is wmopts JSON, so the key appears as "logoKey":"<key>".
func (r *repo) LogoInUse(key string) (bool, error) {
	var n int
	err := r.db.QueryRow(r.db.Rebind(
		"SELECT COUNT(*) FROM jobs WHERE status IN ('pending', 'processing') AND watermark_opts LIKE ?"),
		`%"logoKey":"`+key+`"%`,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("brand: logo in use: %w", err)
	}
	return n > 0, nil
}
