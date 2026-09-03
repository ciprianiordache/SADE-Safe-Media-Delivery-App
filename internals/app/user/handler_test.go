package user

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sade/internals/testutil"
)

func newHandler(t *testing.T) (*Handler, Service) {
	svc := NewService(NewRepo(testutil.DB(t, User{})), testutil.Logger())
	return NewHandler(svc, testutil.Logger()), svc
}

func TestHandlerList(t *testing.T) {
	h, svc := newHandler(t)
	if _, err := svc.EnsureByEmail("a@b.c"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EnsureByEmail("d@e.f"); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	h.List(w, httptest.NewRequest(http.MethodGet, "/api/users", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var out []Response
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, w.Body.String())
	}
	if len(out) != 2 {
		t.Errorf("got %d users, want 2", len(out))
	}
}

func TestHandlerGetNotFound(t *testing.T) {
	h, _ := newHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/users/missing", nil)
	req.SetPathValue("id", "missing")

	w := httptest.NewRecorder()
	h.Get(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHandlerSetRole(t *testing.T) {
	h, svc := newHandler(t)
	u, _ := svc.EnsureByEmail("a@b.c")

	// happy path
	req := httptest.NewRequest(http.MethodPatch, "/api/users/"+u.ID, strings.NewReader(`{"role":"admin"}`))
	req.SetPathValue("id", u.ID)
	w := httptest.NewRecorder()
	h.SetRole(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", w.Code, w.Body.String())
	}
	var got Response
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Role != RoleAdmin {
		t.Errorf("role = %q, want admin", got.Role)
	}

	// unknown role -> 400
	req = httptest.NewRequest(http.MethodPatch, "/api/users/"+u.ID, strings.NewReader(`{"role":"wizard"}`))
	req.SetPathValue("id", u.ID)
	w = httptest.NewRecorder()
	h.SetRole(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}

	// unknown field -> 400
	req = httptest.NewRequest(http.MethodPatch, "/api/users/"+u.ID, strings.NewReader(`{"roleName":"admin"}`))
	req.SetPathValue("id", u.ID)
	w = httptest.NewRecorder()
	h.SetRole(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for unknown field", w.Code)
	}
}
