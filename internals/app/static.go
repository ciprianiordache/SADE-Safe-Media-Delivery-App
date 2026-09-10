package app

import (
	"net/http"
	"os"
	"path"
	"strings"
)

// reservedPrefixes are paths static serving must never answer, even with the
// SPA shell - they belong to other handlers, and an unmatched route under
// one of them (e.g. a typo'd /api/ call) should 404 as a miss, not silently
// render index.html.
var reservedPrefixes = []string{"/api/", "/p/", "/d/", "/healthz"}

// spaFileServer serves a SvelteKit adapter-static build (fallback:
// 'index.html') from dir: a request for a real file (JS/CSS/an image under
// frontend/build) is served as-is; anything else - a client-side route like
// /app/jobs/<id> hit on a hard refresh, or "/" - falls back to index.html so
// SvelteKit's router can take over in the browser.
func spaFileServer(dir string) http.Handler {
	root := http.Dir(dir)
	fileServer := http.FileServer(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, p := range reservedPrefixes {
			if strings.HasPrefix(r.URL.Path, p) {
				http.NotFound(w, r)
				return
			}
		}
		if f, err := root.Open(path.Clean(r.URL.Path)); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}

// hasFrontendBuild reports whether dir looks like a built SvelteKit app, so
// NewRouter can skip registering the static handler (and keep the mux's
// default 404) when the frontend hasn't been built yet.
func hasFrontendBuild(dir string) bool {
	if dir == "" {
		return false
	}
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}
