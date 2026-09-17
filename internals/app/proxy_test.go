package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"sade/internals/ratelimit"
)

var loopbackOnly = []string{"127.0.0.1/32", "::1/128"}

func requestFrom(peer string, headers map[string][]string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = peer
	for k, vs := range headers {
		for _, v := range vs {
			r.Header.Add(k, v)
		}
	}
	return r
}

func TestClientIPIgnoresForwardedHeaderFromUntrustedPeer(t *testing.T) {
	trust := newProxyTrust(loopbackOnly)

	// The whole point: a client that reaches us directly cannot promote
	// itself to another address just by sending the header.
	r := requestFrom("203.0.113.9:54321", map[string][]string{
		"X-Forwarded-For": {"198.51.100.1"},
	})
	if got := trust.clientIP(r); got != "203.0.113.9" {
		t.Fatalf("clientIP = %q, want the real peer 203.0.113.9", got)
	}
}

func TestClientIPUsesForwardedHeaderFromTrustedProxy(t *testing.T) {
	trust := newProxyTrust(loopbackOnly)

	r := requestFrom("127.0.0.1:41234", map[string][]string{
		"X-Forwarded-For": {"198.51.100.1"},
	})
	if got := trust.clientIP(r); got != "198.51.100.1" {
		t.Fatalf("clientIP = %q, want the forwarded client 198.51.100.1", got)
	}
}

func TestClientIPWalksChainFromTheRight(t *testing.T) {
	// Two of our own hops appended after the client; both are trusted, so
	// the furthest believable entry is the client itself.
	trust := newProxyTrust([]string{"127.0.0.1/32", "10.0.0.0/8"})

	r := requestFrom("127.0.0.1:41234", map[string][]string{
		// Several headers, several values each - both are legal.
		"X-Forwarded-For": {"198.51.100.1, 10.0.0.7", "10.0.0.8"},
	})
	if got := trust.clientIP(r); got != "198.51.100.1" {
		t.Fatalf("clientIP = %q, want 198.51.100.1", got)
	}
}

func TestClientIPFallsBackToPeerWhenChainIsAllTrusted(t *testing.T) {
	trust := newProxyTrust(loopbackOnly)

	r := requestFrom("127.0.0.1:41234", map[string][]string{
		"X-Forwarded-For": {"127.0.0.1"},
	})
	if got := trust.clientIP(r); got != "127.0.0.1" {
		t.Fatalf("clientIP = %q, want 127.0.0.1", got)
	}
}

func TestClientIPWithNoTrustedProxiesAlwaysUsesPeer(t *testing.T) {
	trust := newProxyTrust(nil)

	r := requestFrom("127.0.0.1:41234", map[string][]string{
		"X-Forwarded-For": {"198.51.100.1"},
	})
	if got := trust.clientIP(r); got != "127.0.0.1" {
		t.Fatalf("clientIP = %q, want 127.0.0.1", got)
	}
}

func TestSchemeHonoursForwardedProtoOnlyFromTrustedProxy(t *testing.T) {
	trust := newProxyTrust(loopbackOnly)

	cases := []struct {
		name string
		peer string
		hdr  string
		want string
	}{
		{"trusted proxy terminating TLS", "127.0.0.1:1", "https", "https"},
		{"trusted proxy on plain http", "127.0.0.1:1", "http", "http"},
		{"chain keeps the client-most value", "127.0.0.1:1", "https, http", "https"},
		{"untrusted peer cannot claim https", "203.0.113.9:1", "https", "http"},
		{"no header at all", "127.0.0.1:1", "", "http"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := map[string][]string{}
			if tc.hdr != "" {
				h["X-Forwarded-Proto"] = []string{tc.hdr}
			}
			if got := trust.scheme(requestFrom(tc.peer, h)); got != tc.want {
				t.Fatalf("scheme = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSecurityHeadersSendsHSTSOnlyOverHTTPS(t *testing.T) {
	trust := newProxyTrust(loopbackOnly)
	h := SecurityHeaders(trust)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	plain := httptest.NewRecorder()
	h.ServeHTTP(plain, requestFrom("127.0.0.1:1", nil))
	if got := plain.Header().Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("HSTS over plain http = %q, want it omitted", got)
	}
	if got := plain.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := plain.Header().Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
		t.Fatalf("Referrer-Policy = %q", got)
	}

	secure := httptest.NewRecorder()
	h.ServeHTTP(secure, requestFrom("127.0.0.1:1", map[string][]string{
		"X-Forwarded-Proto": {"https"},
	}))
	if got := secure.Header().Get("Strict-Transport-Security"); got == "" {
		t.Fatal("HSTS missing on a request forwarded as https")
	}
}

// The bug this all exists for: behind a terminator every request arrives
// from loopback, so a limiter keyed on the peer would let one client's
// burst lock out everybody else.
func TestRateLimitKeysOnForwardedClientBehindProxy(t *testing.T) {
	trust := newProxyTrust(loopbackOnly)
	limiter := ratelimit.New(2, time.Minute)
	h := RateLimit(limiter, trust)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	send := func(client string) int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, requestFrom("127.0.0.1:41234", map[string][]string{
			"X-Forwarded-For": {client},
		}))
		return rec.Code
	}

	if got := send("198.51.100.1"); got != http.StatusOK {
		t.Fatalf("first request = %d, want 200", got)
	}
	if got := send("198.51.100.1"); got != http.StatusOK {
		t.Fatalf("second request = %d, want 200", got)
	}
	if got := send("198.51.100.1"); got != http.StatusTooManyRequests {
		t.Fatalf("third request from the same client = %d, want 429", got)
	}
	// A different client behind the same proxy still has its own budget.
	if got := send("198.51.100.2"); got != http.StatusOK {
		t.Fatalf("other client = %d, want 200 - the limiter collapsed into one global bucket", got)
	}
}
