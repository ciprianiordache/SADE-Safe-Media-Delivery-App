package user

import (
	"errors"

	"sade/internals/database"

	crud "github.com/ciprianiordache/crud-depot"
)

// Repo is the persistence boundary for accounts.
type Repo interface {
	Create(u *User) (id string, err error)
	GetByID(id string) (*User, error)
	GetByEmail(email string) (*User, error)
	List(offset, limit int) ([]User, error)
	Update(u *User) error
	Delete(id string) error
}

type repo struct {
	db *database.Database
}

// NewRepo builds the account repository over the shared connection. The SQL
// dialect is already decided inside internals/database.
func NewRepo(db *database.Database) Repo { return &repo{db: db} }

func (r *repo) Create(u *User) (string, error) {
	return r.db.CRUD().Create(u)
}

func (r *repo) GetByID(id string) (*User, error) {
	return r.one("id", id)
}

func (r *repo) GetByEmail(email string) (*User, error) {
	return r.one("email", email)
}

func (r *repo) one(field, value string) (*User, error) {
	var u User
	err := r.db.CRUD().ReadOne(User{}, field, value, &u)
	if errors.Is(err, crud.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *repo) List(offset, limit int) ([]User, error) {
	var users []User
	err := r.db.CRUD().Get(User{}, &users, offset, limit)
	if errors.Is(err, crud.ErrNotFound) {
		return []User{}, nil // an empty page is not an error
	}
	return users, err
}

func (r *repo) Update(u *User) error {
	err := r.db.CRUD().Update(u, "id", u.ID)
	if errors.Is(err, crud.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func (r *repo) Delete(id string) error {
	err := r.db.CRUD().Delete(User{}, "id", id)
	if errors.Is(err, crud.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
