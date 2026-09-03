package magic_token

import (
	"errors"
	"testing"
	"time"

	"sade/internals/app/user"
	"sade/internals/testutil"
)

func setup(t *testing.T) (Repo, string) {
	t.Helper()
	db := testutil.DB(t, user.User{}, MagicToken{})
	uid, err := db.CRUD().Create(&user.User{Email: "a@b.c", Role: user.RoleOperator})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return NewRepo(db), uid
}

func TestConsumeHappyPath(t *testing.T) {
	r, uid := setup(t)
	if _, err := r.Create(&MagicToken{UserID: uid, TokenHash: "h1", ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Consume("h1", time.Now())
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if got != uid {
		t.Errorf("userID = %q, want %q", got, uid)
	}

	// second use -> gone
	if _, err := r.Consume("h1", time.Now()); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Consume = %v, want ErrNotFound", err)
	}
}

func TestConsumeExpired(t *testing.T) {
	r, uid := setup(t)
	_, _ = r.Create(&MagicToken{UserID: uid, TokenHash: "h2", ExpiresAt: time.Now().Add(-time.Second)})

	if _, err := r.Consume("h2", time.Now()); !errors.Is(err, ErrExpired) {
		t.Errorf("Consume(expired) = %v, want ErrExpired", err)
	}
	// expired token is left in place (not consumed); a housekeeping job
	// would remove it. Confirm it's still there.
	var mt MagicToken
	if err := r.(*repo).db.CRUD().ReadOne(MagicToken{}, "token_hash", "h2", &mt); err != nil {
		t.Errorf("expired token should remain: %v", err)
	}
}

func TestConsumeUnknown(t *testing.T) {
	r, _ := setup(t)
	if _, err := r.Consume("nope", time.Now()); !errors.Is(err, ErrNotFound) {
		t.Errorf("Consume(unknown) = %v, want ErrNotFound", err)
	}
}

func TestCascadeFromUser(t *testing.T) {
	db := testutil.DB(t, user.User{}, MagicToken{})
	uid, _ := db.CRUD().Create(&user.User{Email: "z@z.z", Role: user.RoleOperator})
	r := NewRepo(db)
	_, _ = r.Create(&MagicToken{UserID: uid, TokenHash: "h3", ExpiresAt: time.Now().Add(time.Minute)})

	if err := db.CRUD().Delete(user.User{}, "id", uid); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if _, err := r.Consume("h3", time.Now()); !errors.Is(err, ErrNotFound) {
		t.Errorf("token survived user delete: %v", err)
	}
}
