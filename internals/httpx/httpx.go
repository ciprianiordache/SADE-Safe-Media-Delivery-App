// Package httpx holds the small HTTP helpers every feature handler shares:
// JSON responses, request decoding with a size cap, and a consistent error
// body. It imports nothing else in the project, so any package can use it
// without creating an import cycle.
package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
)

// ErrorBody is the JSON shape of every error response: {"error": "..."}.
type ErrorBody struct {
	Error string `json:"error"`
}

// WriteJSON writes v as JSON with the given status. A nil v writes just the
// status line and header.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// Error writes {"error": msg} with the given status.
func Error(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, ErrorBody{Error: msg})
}

// DecodeJSON reads a single JSON value from the request body into dst,
// rejecting unknown fields, trailing data, and bodies larger than maxBytes.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("httpx: body must contain a single JSON value")
	}
	return nil
}

// Page reads ?offset= and ?limit= with sane defaults and caps. limit defaults
// to defLimit and is clamped to [1, maxLimit]; offset is clamped to >= 0.
func Page(r *http.Request, defLimit, maxLimit int) (offset, limit int) {
	limit = defLimit
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil {
		limit = v
	}
	if limit < 1 {
		limit = 1
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v > 0 {
		offset = v
	}
	return offset, limit
}
