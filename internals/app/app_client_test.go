package app

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"sade/internals/app/job"
)

// appRequest is a request as the installed app sends it: no cookies, its
// own binding value in a header, the session as a bearer token.
func appRequest(t *testing.T, method, url, body, nonce, bearer string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-SADE-Client", "app")
	if nonce != "" {
		req.Header.Set("X-SADE-Login", nonce)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

var sixDigits = regexp.MustCompile(`\b\d{6}\b`)

// The installed app signs in with the emailed 6-digit code, bound to its own
// header value instead of a cookie, gets the session back as a token, and
// plays its media through short-lived signed links.
func TestAppClientSignInAndMedia(t *testing.T) {
	srv, _, cm, _ := buildTestApp(t)
	const nonce = "app-binding-nonce-0123456789abcdefABCDEF"

	resp := appRequest(t, "POST", srv.URL+"/api/auth/request", `{"email":"op@example.com"}`, nonce, "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("request = %d, want 202", resp.StatusCode)
	}
	cm.mu.Lock()
	code := sixDigits.FindString(cm.sent[len(cm.sent)-1].Text)
	cm.mu.Unlock()
	if code == "" {
		t.Fatal("no 6-digit code in the email")
	}

	// Another device's binding value cannot spend the code.
	resp = appRequest(t, "POST", srv.URL+"/api/auth/code", `{"email":"op@example.com","code":"`+code+`"}`, "some-other-app-nonce-0000000000", "")
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("code accepted with a different binding value")
	}

	resp = appRequest(t, "POST", srv.URL+"/api/auth/code", `{"email":"op@example.com","code":"`+code+`"}`, nonce, "")
	var signed struct {
		Status string `json:"status"`
		Token  string `json:"token"`
	}
	json.NewDecoder(resp.Body).Decode(&signed)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || signed.Status != "signed_in" || signed.Token == "" {
		t.Fatalf("code sign-in = %d %+v, want a token", resp.StatusCode, signed)
	}

	// The bearer token is the session.
	me := appRequest(t, "GET", srv.URL+"/api/me", "", "", signed.Token)
	me.Body.Close()
	if me.StatusCode != http.StatusOK {
		t.Fatalf("/api/me with bearer = %d, want 200", me.StatusCode)
	}

	// Upload with the token, then open the original through a signed link.
	body, ct := multipartUpload(t, "clip.png", []byte("original-bytes"), map[string]string{"recipientEmail": "client@example.com"})
	up, _ := http.NewRequest("POST", srv.URL+"/api/jobs", body)
	up.Header.Set("Content-Type", ct)
	up.Header.Set("Authorization", "Bearer "+signed.Token)
	uresp, err := http.DefaultClient.Do(up)
	if err != nil {
		t.Fatal(err)
	}
	var created job.Response
	json.NewDecoder(uresp.Body).Decode(&created)
	uresp.Body.Close()
	if uresp.StatusCode != http.StatusCreated {
		t.Fatalf("upload with bearer = %d, want 201", uresp.StatusCode)
	}
	detail := appRequest(t, "GET", srv.URL+"/api/jobs/"+created.ID, "", "", signed.Token)
	var full job.Response
	json.NewDecoder(detail.Body).Decode(&full)
	detail.Body.Close()
	if len(full.Assets) == 0 {
		t.Fatal("job has no assets")
	}
	assetID := full.Assets[0].ID

	lresp := appRequest(t, "GET", srv.URL+"/api/jobs/"+created.ID+"/assets/"+assetID+"/link", "", "", signed.Token)
	var link struct {
		URL string `json:"url"`
	}
	json.NewDecoder(lresp.Body).Decode(&link)
	lresp.Body.Close()
	if lresp.StatusCode != http.StatusOK || !strings.Contains(link.URL, "?t=") {
		t.Fatalf("link = %d %q", lresp.StatusCode, link.URL)
	}

	media, err := http.Get(srv.URL + link.URL) // no cookie, no token: the link alone
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(media.Body)
	media.Body.Close()
	if media.StatusCode != http.StatusOK || string(got) != "original-bytes" {
		t.Fatalf("signed media = %d %q", media.StatusCode, got)
	}

	// The link names its asset: it opens nothing else, and nothing opens
	// without it.
	other := strings.Replace(link.URL, "/assets/"+assetID+"/", "/assets/not-this-asset/", 1)
	if r, _ := http.Get(srv.URL + other); r.StatusCode != http.StatusNotFound {
		t.Fatalf("link on another asset = %d, want 404", r.StatusCode)
	}
	plain := strings.SplitN(link.URL, "?", 2)[0]
	if r, _ := http.Get(srv.URL + plain); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("content without session or link = %d, want 401", r.StatusCode)
	}

	// Sign-out ends the bearer session.
	appRequest(t, "POST", srv.URL+"/api/auth/logout", "", "", signed.Token).Body.Close()
	me = appRequest(t, "GET", srv.URL+"/api/me", "", "", signed.Token)
	me.Body.Close()
	if me.StatusCode != http.StatusUnauthorized {
		t.Fatalf("/api/me after logout = %d, want 401", me.StatusCode)
	}
}

// The app's origin gets CORS with the headers it sends, and passes the
// origin check on a state-changing request.
func TestAppOriginCORS(t *testing.T) {
	srv, _, _, _ := buildTestApp(t)
	req, _ := http.NewRequest("OPTIONS", srv.URL+"/api/auth/request", nil)
	req.Header.Set("Origin", "tauri://localhost")
	req.Header.Set("Access-Control-Request-Method", "POST")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.Header.Get("Access-Control-Allow-Origin") != "tauri://localhost" ||
		!strings.Contains(resp.Header.Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Fatalf("preflight headers = %v", resp.Header)
	}

	post, _ := http.NewRequest("POST", srv.URL+"/api/auth/logout", nil)
	post.Header.Set("Origin", "tauri://localhost")
	post.Header.Set("Sec-Fetch-Site", "cross-site")
	pr, err := http.DefaultClient.Do(post)
	if err != nil {
		t.Fatal(err)
	}
	pr.Body.Close()
	if pr.StatusCode == http.StatusForbidden {
		t.Fatal("app origin refused by the origin check")
	}
}
