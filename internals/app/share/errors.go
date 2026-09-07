// Package share serves the public, login-free links to a job's watermarked
// preview: GET /p/{token} streams it inline, GET /d/{token} sends it as a
// download. The token is a stateless HMAC capability minted by
// internals/token (purpose "preview" for /p, "download" for /d) whose subject
// is the preview asset's id - there is no session and no database row for the
// link itself.
package share

import "errors"

var (
	// errNotFound collapses every "the caller gets a 404" case - bad or
	// unknown token, wrong purpose, missing asset, asset that is not a
	// preview, missing blob - so a probe cannot tell them apart.
	errNotFound = errors.New("share: not found")
	// errGone is a token that verified but has expired (410).
	errGone = errors.New("share: link expired")
)
