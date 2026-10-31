package app

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sade/internals/testutil"
	"strings"
	"testing"
	"time"
)

func TestOriginGuard(t *testing.T) {
	g := newOriginGuard([]string{"https://sade.example/", "http://localhost:5173"}, "/api/payments/webhook")

	cases := []struct {
		name, method, path, origin, fetchSite string
		want                                  bool
	}{
		{"safe method always passes", "GET", "/api/jobs", "https://evil.example", "cross-site", true},
		{"same-origin fetch", "POST", "/api/jobs", "https://sade.example", "same-origin", true},
		{"allowed dev origin, same-site", "POST", "/api/jobs", "http://localhost:5173", "same-site", true},
		{"cross-site page", "POST", "/api/jobs", "https://evil.example", "cross-site", false},
		{"sibling subdomain", "POST", "/api/jobs", "https://evil.sade.example", "same-site", false},
		{"old browser, foreign origin", "POST", "/api/jobs", "https://evil.example", "", false},
		{"sandboxed frame", "POST", "/api/auth/complete", "null", "", false},
		{"non-browser client", "POST", "/api/auth/request", "", "", true},
		{"webhook exempt", "POST", "/api/payments/webhook", "https://evil.example", "cross-site", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(c.method, c.path, nil)
			if c.origin != "" {
				r.Header.Set("Origin", c.origin)
			}
			if c.fetchSite != "" {
				r.Header.Set("Sec-Fetch-Site", c.fetchSite)
			}
			if got := g.allows(r); got != c.want {
				t.Errorf("allows = %v, want %v", got, c.want)
			}
		})
	}
}

func TestCrossOriginUploadIsRefused(t *testing.T) {
	srv, client, cm, _ := buildTestApp(t)
	signIn(t, srv, client, cm, "op@example.com")

	body, ctype := multipartUpload(t, "clip.mp4", []byte("data"), map[string]string{"recipientEmail": "c@example.com"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/jobs", body)
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-site upload = %d, want 403", resp.StatusCode)
	}
}

func TestLogoutEverywhere(t *testing.T) {
	srv, client, cm, _ := buildTestApp(t)
	signIn(t, srv, client, cm, "op@example.com")

	// A second device signed in to the same account.
	jar, _ := cookiejar.New(nil)
	second := &http.Client{Jar: jar, CheckRedirect: client.CheckRedirect}
	signIn(t, srv, second, cm, "op@example.com")

	resp, err := client.Post(srv.URL+"/api/auth/logout-all", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout-all = %d, want 204", resp.StatusCode)
	}
	for name, c := range map[string]*http.Client{"this device": client, "other device": second} {
		if code := get(t, c, srv.URL+"/api/me"); code != http.StatusUnauthorized {
			t.Errorf("%s still signed in after logout-all: %d", name, code)
		}
	}
}

func TestBuildCSPHashesInlineScripts(t *testing.T) {
	dir := t.TempDir()
	html := `<html><head><script>console.log("boot")</script><script src="/x.js"></script></head></html>`
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
	csp := buildCSP(dir)
	// sha256 of `console.log("boot")`
	if !strings.Contains(csp, "'sha256-") || strings.Count(csp, "'sha256-") != 1 {
		t.Errorf("want exactly one inline-script hash: %s", csp)
	}
	for _, want := range []string{"frame-ancestors 'none'", "object-src 'none'", "https://js.stripe.com"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP missing %q: %s", want, csp)
		}
	}
	if strings.Contains(csp, "script-src 'self' 'unsafe-inline'") {
		t.Errorf("scripts must not allow unsafe-inline: %s", csp)
	}
}

func TestRedactPath(t *testing.T) {
	for in, want := range map[string]string{
		"/p/abc.def":       "/p/***",
		"/o/abc.def":       "/o/***",
		"/preview/abc.def": "/preview/***",
		"/api/jobs/123":    "/api/jobs/123",
		"/p/":              "/p/",
	} {
		if got := redactPath(in); got != want {
			t.Errorf("redactPath(%q) = %q, want %q", in, got, want)
		}
	}
}

// The server-wide WriteTimeout would cut a long file stream; LongRequest
// must lift it for its route, even behind RequestLog's statusWriter.
func TestLongRequestOutlivesServerWriteTimeout(t *testing.T) {
	slow := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(400 * time.Millisecond)
		_, _ = w.Write([]byte("done"))
	})
	mux := http.NewServeMux()
	mux.Handle("/short", slow)
	mux.Handle("/long", LongRequest(time.Minute)(slow))

	srv := httptest.NewUnstartedServer(chain(mux, RequestLog(testutil.Logger(), newProxyTrust(nil))))
	srv.Config.WriteTimeout = 200 * time.Millisecond
	srv.Start()
	defer srv.Close()

	if resp, err := http.Get(srv.URL + "/short"); err == nil {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if string(body) == "done" {
			t.Fatal("control route should have been cut by WriteTimeout")
		}
	}
	resp, err := http.Get(srv.URL + "/long")
	if err != nil {
		t.Fatalf("long route cut off: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "done" {
		t.Fatalf("long route body = %q, want done", body)
	}
}
