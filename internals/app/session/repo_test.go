package session

import (
	"errors"
	"testing"
	"time"

	"sade/internals/app/user"
	"sade/internals/testutil"
)

func setup(t *testing.T) (Repo, string) {
	t.Helper()
	db := testutil.DB(t, user.User{}, Session{})
	uid, err := db.CRUD().Create(&user.User{Email: "a@b.c", Role: user.RoleOperator})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return NewRepo(db), uid
}

func TestSessionCreateGetDelete(t *testing.T) {
	r, uid := setup(t)
	exp := time.Now().Add(time.Hour)

	id, err := r.Create(&Session{UserID: uid, TokenHash: "sh1", ExpiresAt: exp})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id == "" {
		t.Fatal("empty id")
	}

	got, err := r.GetByHash("sh1")
	if err != nil {
		t.Fatalf("GetByHash: %v", err)
	}
	if got.UserID != uid || got.Expired(time.Now()) {
		t.Errorf("got %+v", got)
	}

	if _, err := r.GetByHash("missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetByHash(missing) = %v, want ErrNotFound", err)
	}

	if err := r.Delete("sh1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.GetByHash("sh1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetByHash after Delete = %v, want ErrNotFound", err)
	}
	if err := r.Delete("sh1"); err != nil {
		t.Errorf("Delete of unknown hash should be nil, got %v", err)
	}
}

func TestSessionExpiredHelper(t *testing.T) {
	s := Session{ExpiresAt: time.Now().Add(-time.Minute)}
	if !s.Expired(time.Now()) {
		t.Error("Expired should be true for a past ExpiresAt")
	}
}
