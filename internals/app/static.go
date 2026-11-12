package app

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// reservedPrefixes are paths static serving must never answer, even with the
// SPA shell - they belong to other handlers, and an unmatched route under
// one of them (e.g. a typo'd /api/ call) should 404 as a miss, not silently
// render index.html.
var reservedPrefixes = []string{"/api/", "/p/", "/d/", "/o/", "/healthz"}

// spaFileServer serves a SvelteKit adapter-static build from dir: a request
// for a real file (JS/CSS/an image under frontend/build, or "/" -> the static
// marketing page index.html) is served as-is; anything else - a client-side
// route like /app/jobs/<id> hit on a hard refresh - falls back to the SPA
// shell so SvelteKit's router can take over in the browser.
func spaFileServer(dir string) http.Handler {
	root := http.Dir(dir)
	fileServer := http.FileServer(root)
	shell := shellPath(dir)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, p := range reservedPrefixes {
			if strings.HasPrefix(r.URL.Path, p) {
				http.NotFound(w, r)
				return
			}
		}
		// Hashed build output never changes under the same name, so it can be
		// cached for good. Everything else - above all the index.html shell -
		// must be revalidated every time: the CSP header pins the hashes of the
		// shell's inline scripts, and a browser that heuristically cached an
		// old shell next to a newer CSP (or the reverse) renders a blank page.
		if strings.HasPrefix(r.URL.Path, "/_app/immutable/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		if f, err := root.Open(path.Clean(r.URL.Path)); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = shell
		fileServer.ServeHTTP(w, r2)
	})
}

// shellFile is adapter-static's fallback page. The build names it 200.html so
// that index.html can be the static marketing page; a build without it (an
// older one, or a test fixture) falls back to index.html as the shell.
const shellFile = "200.html"

// shellPath is the URL path of the SPA shell inside dir.
func shellPath(dir string) string {
	if info, err := os.Stat(filepath.Join(dir, shellFile)); err == nil && !info.IsDir() {
		return "/" + shellFile
	}
	return "/"
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
