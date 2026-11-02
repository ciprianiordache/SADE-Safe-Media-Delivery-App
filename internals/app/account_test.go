package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"sade/internals/app/auth"
)

// The account-security routes ride the real router: session-guarded,
// scoped to the caller, and revoking the current session signs it out.
func TestAccountSessionsThroughRouter(t *testing.T) {
	srv, client, cm, _ := buildTestApp(t)

	resp, err := http.Get(srv.URL + "/api/account/sessions")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("signed-out sessions = %d, want 401", resp.StatusCode)
	}

	signIn(t, srv, client, cm, "op@example.com")

	resp, err = client.Get(srv.URL + "/api/account/sessions")
	if err != nil {
		t.Fatal(err)
	}
	var list []auth.SessionView
	_ = json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	if len(list) != 1 || !list[0].Current {
		t.Fatalf("sessions = %+v, want one current session", list)
	}

	// A wrong code is a plain 400, whatever the address.
	resp, err = client.Post(srv.URL+"/api/auth/code", "application/json",
		strings.NewReader(`{"email":"op@example.com","code":"123456"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("wrong code = %d, want 400", resp.StatusCode)
	}

	// Revoking the current session signs this browser out.
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/account/sessions/"+list[0].ID, nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke current = %d, want 204", resp.StatusCode)
	}
	resp, _ = client.Get(srv.URL + "/api/me")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("/api/me after revoking the current session = %d, want 401", resp.StatusCode)
	}
}
