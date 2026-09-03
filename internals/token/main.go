// Package token mints and verifies stateless, HMAC-signed capability tokens
// for the public share links (/p/:token to view a preview, /d/:token to
// download it). A token carries a subject (an asset id) and an expiry, signed
// with Auth.HMACSecret - there is no database row and no login.
//
// The same secret also protects sessions, so every use is domain-separated:
// the signed payload is prefixed with a purpose string ("preview",
// "download", ...). A token minted for one purpose never verifies for
// another.
package token

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrMalformed is returned when a token is not well-formed base64/JSON-ish
	// structure this package produces.
	ErrMalformed = errors.New("token: malformed")
	// ErrBadSignature is returned when the HMAC does not match (wrong secret,
	// tampered payload, or wrong purpose).
	ErrBadSignature = errors.New("token: signature mismatch")
	// ErrExpired is returned when the token's deadline has passed.
	ErrExpired = errors.New("token: expired")
)

// Signer mints and verifies tokens with one secret.
type Signer struct {
	secret []byte
}

// New returns a Signer over secret (typically config.Auth.HMACSecret). It
// errors on an empty secret so a misconfigured deployment fails loudly.
func New(secret string) (*Signer, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("token: empty secret")
	}
	return &Signer{secret: []byte(secret)}, nil
}

// Sign returns a token that authorises purpose on subject until it expires
// ttl from now. purpose and subject must be non-empty and contain no '.'
// (the field separator).
func (s *Signer) Sign(purpose, subject string, ttl time.Duration) (string, error) {
	if purpose == "" || subject == "" {
		return "", errors.New("token: purpose and subject are required")
	}
	if strings.ContainsRune(purpose, '.') || strings.ContainsRune(subject, '.') {
		return "", errors.New("token: purpose and subject must not contain '.'")
	}
	exp := time.Now().Add(ttl).Unix()
	payload := fmt.Sprintf("%s.%s.%d", purpose, subject, exp)
	body := b64(payload)
	return body + "." + b64Raw(s.mac(body)), nil
}

// Verify checks the token's signature and expiry and that it was minted for
// purpose, returning the subject it authorises.
func (s *Signer) Verify(purpose, tok string) (subject string, err error) {
	body, sig, ok := strings.Cut(tok, ".")
	if !ok || body == "" || sig == "" {
		return "", ErrMalformed
	}
	want, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return "", ErrMalformed
	}
	if !hmac.Equal(want, s.mac(body)) {
		return "", ErrBadSignature
	}

	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return "", ErrMalformed
	}
	parts := strings.Split(string(raw), ".")
	if len(parts) != 3 {
		return "", ErrMalformed
	}
	gotPurpose, gotSubject, expStr := parts[0], parts[1], parts[2]
	if gotPurpose != purpose {
		// A valid signature for a different purpose is still a rejection.
		return "", ErrBadSignature
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return "", ErrMalformed
	}
	if time.Now().After(time.Unix(exp, 0)) {
		return "", ErrExpired
	}
	return gotSubject, nil
}

func (s *Signer) mac(body string) []byte {
	h := hmac.New(sha256.New, s.secret)
	h.Write([]byte(body))
	return h.Sum(nil)
}

func b64(s string) string    { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
func b64Raw(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
