package app

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"sade/internals/httpx"
)

// --- CSRF: same-origin check for state-changing requests -------------------

// originGuard rejects a state-changing request (POST/PUT/PATCH/DELETE) that
// a browser sent from a page SADE does not serve. SameSite=Lax already keeps
// the session cookie off most cross-site requests; this is the second layer
// for the cases it misses (a sibling subdomain counts as "same-site", and the
// public payment routes have no cookie to protect at all).
//
// A browser always says where a request came from - Sec-Fetch-Site in every
// current one, Origin on every cross-origin POST - so a request carrying
// neither is not a browser-driven forgery (curl, Stripe's webhook) and is
// let through; authentication still applies to it as usual.
type originGuard struct {
	allowed []string // exact origins: scheme://host[:port]
	exempt  []string // path prefixes called server-to-server (webhooks)
}

func newOriginGuard(origins []string, exempt ...string) originGuard {
	var allowed []string
	for _, o := range origins {
		if n := normalizeOrigin(o); n != "" && !slices.Contains(allowed, n) {
			allowed = append(allowed, n)
		}
	}
	return originGuard{allowed: allowed, exempt: exempt}
}

// normalizeOrigin reduces a URL to its origin ("https://a.b:8443"), or "".
func normalizeOrigin(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

func (g originGuard) allows(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	for _, p := range g.exempt {
		if strings.HasPrefix(r.URL.Path, p) {
			return true
		}
	}
	origin := strings.ToLower(strings.TrimSpace(r.Header.Get("Origin")))
	if origin != "" && origin != "null" && slices.Contains(g.allowed, origin) {
		return true
	}
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return true
	case "same-site", "cross-site":
		return false
	}
	// No Sec-Fetch-Site: an older browser or a non-browser client. An Origin
	// we don't recognise (including "null", from sandboxed frames) is a
	// cross-origin page; no Origin at all is not a browser form or fetch.
	return origin == ""
}

func OriginCheck(g originGuard) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !g.allows(r) {
				httpx.Error(w, http.StatusForbidden, "cross-origin request refused")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// --- Content-Security-Policy ------------------------------------------------

var inlineScriptRe = regexp.MustCompile(`(?s)<script(\s[^>]*)?>(.*?)</script>`)

// buildCSP returns the policy for every response. Scripts are limited to our
// own origin, Stripe.js, and the exact inline bootstrap script(s) the
// SvelteKit build put in index.html (by hash, read once at startup, so a
// rebuilt frontend is picked up on restart). Styles keep 'unsafe-inline':
// Svelte writes style attributes, and a style cannot run code.
func buildCSP(frontendDir string) string {
	scriptSrc := []string{"'self'", "https://js.stripe.com", "https://*.js.stripe.com"}
	if b, err := os.ReadFile(filepath.Join(frontendDir, "index.html")); err == nil {
		for _, m := range inlineScriptRe.FindAllStringSubmatch(string(b), -1) {
			if strings.Contains(m[1], "src=") || strings.TrimSpace(m[2]) == "" {
				continue
			}
			sum := sha256.Sum256([]byte(m[2]))
			scriptSrc = append(scriptSrc, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
		}
	}
	return strings.Join([]string{
		"default-src 'self'",
		"script-src " + strings.Join(scriptSrc, " "),
		"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
		"font-src 'self' https://fonts.gstatic.com",
		"img-src 'self' data: blob: https://*.stripe.com",
		"media-src 'self' blob:",
		"connect-src 'self' https://api.stripe.com https://*.stripe.com",
		"frame-src https://js.stripe.com https://*.js.stripe.com https://hooks.stripe.com",
		"frame-ancestors 'none'",
		"base-uri 'self'",
		"form-action 'self'",
		"object-src 'none'",
	}, "; ")
}

// --- log redaction ----------------------------------------------------------

// capabilityPrefixes are routes whose path segment after the prefix is a
// signed capability token - holding the URL is the authorization, so it
// must never land in a log file.
var capabilityPrefixes = []string{"/p/", "/d/", "/o/", "/preview/"}

// redactPath replaces the token in a capability URL with "***".
func redactPath(p string) string {
	for _, pre := range capabilityPrefixes {
		if strings.HasPrefix(p, pre) && len(p) > len(pre) {
			return pre + "***"
		}
	}
	return p
}
