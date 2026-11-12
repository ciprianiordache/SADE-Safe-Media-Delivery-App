package app

import (
	"net/http"
	"strings"
	"time"

	"sade/internals/app/job"
	"sade/internals/httpx"
	"sade/internals/token"
)

// assetLinkTTL bounds a media link the installed app plays a file with. A
// player keeps asking for byte ranges for as long as the video runs, so it
// outlives a long watch; the app asks for a fresh one on every open.
const assetLinkTTL = 6 * time.Hour

// mediaAccess serves the operator's own media either with the session (the
// browser) or with a signed "asset" link (?t=) minted by /link for that same
// operator and asset - an <img> or <video> in the installed app cannot send
// the bearer token. A link names its asset, so it opens nothing else.
func mediaAccess(s *token.Signer, content http.HandlerFunc, session http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t := r.URL.Query().Get("t")
		if t == "" || s == nil {
			session.ServeHTTP(w, r)
			return
		}
		subject, err := s.Verify("asset", t)
		uid, assetID, ok := strings.Cut(subject, "|")
		if err != nil || !ok || uid == "" || assetID != r.PathValue("assetId") {
			httpx.Error(w, http.StatusNotFound, "not found")
			return
		}
		content(w, r.WithContext(job.WithUserID(r.Context(), uid)))
	})
}

// signAsset is the subject an asset link carries: who may open it, and what.
func signAsset(s *token.Signer) func(userID, assetID string) (string, error) {
	return func(userID, assetID string) (string, error) {
		return s.Sign("asset", userID+"|"+assetID, assetLinkTTL)
	}
}

// downloads serves the installers and the updater's latest.json from dir.
// No directory listings; latest.json is never cached, so a new release shows
// at once. A missing dir answers 404 like any unknown file.
func downloads(dir string) http.Handler {
	files := http.StripPrefix("/downloads/", http.FileServer(http.Dir(dir)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, ".json") {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}
