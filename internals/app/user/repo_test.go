package user

import (
	"errors"
	"testing"

	"sade/internals/testutil"
)

func TestRepoCRUD(t *testing.T) {
	r := NewRepo(testutil.DB(t, User{}))

	id, err := r.Create(&User{Email: "a@b.c", Role: RoleOperator})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id == "" {
		t.Fatal("Create returned empty id")
	}

	got, err := r.GetByID(id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Email != "a@b.c" || got.Role != RoleOperator {
		t.Errorf("got %+v", got)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Errorf("timestamps not populated: %+v", got)
	}

	byEmail, err := r.GetByEmail("a@b.c")
	if err != nil || byEmail.ID != id {
		t.Errorf("GetByEmail = %+v, %v", byEmail, err)
	}

	if _, err := r.GetByID("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetByID(missing) = %v, want ErrNotFound", err)
	}
	if _, err := r.GetByEmail("nobody@x.y"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetByEmail(missing) = %v, want ErrNotFound", err)
	}

	got.Role = RoleAdmin
	if err := r.Update(got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if reread, _ := r.GetByID(id); reread.Role != RoleAdmin {
		t.Errorf("role not persisted: %q", reread.Role)
	}

	if err := r.Update(&User{ID: "nope", Email: "z@z.z"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update(missing) = %v, want ErrNotFound", err)
	}

	list, err := r.List(0, 10)
	if err != nil || len(list) != 1 {
		t.Errorf("List = %d rows, %v", len(list), err)
	}

	if err := r.Delete(id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.GetByID(id); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetByID after Delete = %v, want ErrNotFound", err)
	}
	if err := r.Delete("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete(missing) = %v, want ErrNotFound", err)
	}
	if empty, err := r.List(0, 10); err != nil || len(empty) != 0 {
		t.Errorf("List after delete = %d rows, %v", len(empty), err)
	}
}
