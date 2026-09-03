package user

import (
	"errors"
	"testing"

	"sade/internals/testutil"
)

func newSvc(t *testing.T) Service {
	return NewService(NewRepo(testutil.DB(t, User{})), testutil.Logger())
}

func TestEnsureByEmailCreatesThenReturnsSame(t *testing.T) {
	s := newSvc(t)

	first, err := s.EnsureByEmail("  Op@Example.COM ")
	if err != nil {
		t.Fatalf("EnsureByEmail: %v", err)
	}
	if first.Email != "op@example.com" {
		t.Errorf("email not normalised: %q", first.Email)
	}
	if first.Role != RoleOperator {
		t.Errorf("role = %q, want operator", first.Role)
	}

	again, err := s.EnsureByEmail("op@example.com")
	if err != nil {
		t.Fatalf("EnsureByEmail (2nd): %v", err)
	}
	if again.ID != first.ID {
		t.Errorf("second call created a new account: %q != %q", again.ID, first.ID)
	}
}

func TestEnsureByEmailRejectsBadAddress(t *testing.T) {
	s := newSvc(t)
	for _, bad := range []string{"", "   ", "not-an-email", "a@", "@b.c"} {
		if _, err := s.EnsureByEmail(bad); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("EnsureByEmail(%q) = %v, want ErrInvalidInput", bad, err)
		}
	}
}

func TestSetRole(t *testing.T) {
	s := newSvc(t)
	u, _ := s.EnsureByEmail("a@b.c")

	promoted, err := s.SetRole(u.ID, RoleAdmin)
	if err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	if promoted.Role != RoleAdmin {
		t.Errorf("role = %q, want admin", promoted.Role)
	}

	if _, err := s.SetRole(u.ID, "wizard"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("SetRole(bad) = %v, want ErrInvalidInput", err)
	}
	if _, err := s.SetRole("missing-id", RoleAdmin); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetRole(missing) = %v, want ErrNotFound", err)
	}
}

func TestGetByIDNotFound(t *testing.T) {
	if _, err := newSvc(t).GetByID("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetByID(missing) = %v, want ErrNotFound", err)
	}
}
