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
	if _, err := r.Create(&MagicToken{UserID: uid, TokenHash: "h1", BrowserHash: "b1", ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Consume("h1", "b1", time.Now())
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if got != uid {
		t.Errorf("userID = %q, want %q", got, uid)
	}

	// second use -> gone
	if _, err := r.Consume("h1", "b1", time.Now()); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Consume = %v, want ErrNotFound", err)
	}
}

func TestConsumeExpired(t *testing.T) {
	r, uid := setup(t)
	_, _ = r.Create(&MagicToken{UserID: uid, TokenHash: "h2", ExpiresAt: time.Now().Add(-time.Second)})

	if _, err := r.Consume("h2", "b", time.Now()); !errors.Is(err, ErrExpired) {
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
	if _, err := r.Consume("nope", "b", time.Now()); !errors.Is(err, ErrNotFound) {
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
	if _, err := r.Consume("h3", "b", time.Now()); !errors.Is(err, ErrNotFound) {
		t.Errorf("token survived user delete: %v", err)
	}
}

func TestConsumeOtherBrowserKeepsToken(t *testing.T) {
	r, uid := setup(t)
	_, _ = r.Create(&MagicToken{UserID: uid, TokenHash: "h4", BrowserHash: "mine", ExpiresAt: time.Now().Add(time.Minute)})

	if _, err := r.Consume("h4", "theirs", time.Now()); !errors.Is(err, ErrOtherBrowser) {
		t.Fatalf("Consume(other browser) = %v, want ErrOtherBrowser", err)
	}
	// Still usable from the right browser.
	if got, err := r.Consume("h4", "mine", time.Now()); err != nil || got != uid {
		t.Fatalf("Consume(right browser) = %q, %v", got, err)
	}
}

func TestConsumeByCode(t *testing.T) {
	r, uid := setup(t)
	exp := time.Now().Add(time.Minute)
	_, _ = r.Create(&MagicToken{UserID: uid, TokenHash: "t1", BrowserHash: "b", CodeHash: "c1", ExpiresAt: exp})
	_, _ = r.Create(&MagicToken{UserID: uid, TokenHash: "t2", BrowserHash: "b", CodeHash: "c2", ExpiresAt: exp})
	is := func(code string) func(MagicToken) bool {
		return func(t MagicToken) bool { return t.CodeHash == code }
	}

	// Another browser never matches, even with the right code.
	if err := r.ConsumeByCode(uid, "other", is("c1"), 5, time.Now()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other browser = %v, want ErrNotFound", err)
	}
	if err := r.ConsumeByCode(uid, "b", is("nope"), 5, time.Now()); !errors.Is(err, ErrBadCode) {
		t.Fatalf("wrong code = %v, want ErrBadCode", err)
	}
	// The older email's code still works, and spends both emails.
	if err := r.ConsumeByCode(uid, "b", is("c1"), 5, time.Now()); err != nil {
		t.Fatalf("right code: %v", err)
	}
	if _, err := r.Consume("t2", "b", time.Now()); !errors.Is(err, ErrNotFound) {
		t.Errorf("sibling link survived a code sign-in: %v", err)
	}
}

func TestConsumeByCodeLocksAfterMaxAttempts(t *testing.T) {
	r, uid := setup(t)
	_, _ = r.Create(&MagicToken{UserID: uid, TokenHash: "t", BrowserHash: "b", CodeHash: "right", ExpiresAt: time.Now().Add(time.Minute)})
	wrong := func(MagicToken) bool { return false }
	for i := 0; i < 3; i++ {
		if err := r.ConsumeByCode(uid, "b", wrong, 3, time.Now()); !errors.Is(err, ErrBadCode) {
			t.Fatalf("attempt %d = %v", i+1, err)
		}
	}
	right := func(t MagicToken) bool { return t.CodeHash == "right" }
	if err := r.ConsumeByCode(uid, "b", right, 3, time.Now()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after lockout = %v, want ErrNotFound", err)
	}
}
