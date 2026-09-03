package token

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	s, err := New("super-secret-value")
	if err != nil {
		t.Fatal(err)
	}
	tok, err := s.Sign("preview", "asset-123", time.Hour)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	sub, err := s.Verify("preview", tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if sub != "asset-123" {
		t.Errorf("subject = %q, want asset-123", sub)
	}
}

func TestVerifyRejectsWrongPurpose(t *testing.T) {
	s, _ := New("secret")
	tok, _ := s.Sign("preview", "a1", time.Hour)
	if _, err := s.Verify("download", tok); !errors.Is(err, ErrBadSignature) {
		t.Errorf("cross-purpose verify: err = %v, want ErrBadSignature", err)
	}
}

func TestVerifyRejectsWrongSecret(t *testing.T) {
	a, _ := New("secret-a")
	b, _ := New("secret-b")
	tok, _ := a.Sign("preview", "a1", time.Hour)
	if _, err := b.Verify("preview", tok); !errors.Is(err, ErrBadSignature) {
		t.Errorf("foreign-secret verify: err = %v, want ErrBadSignature", err)
	}
}

func TestVerifyRejectsTamperedPayload(t *testing.T) {
	s, _ := New("secret")
	tok, _ := s.Sign("preview", "a1", time.Hour)
	body, sig, _ := strings.Cut(tok, ".")
	// flip a character in the payload segment
	mangled := "x" + body[1:] + "." + sig
	if _, err := s.Verify("preview", mangled); err == nil {
		t.Error("tampered token verified")
	}
}

func TestVerifyRejectsExpired(t *testing.T) {
	s, _ := New("secret")
	tok, _ := s.Sign("preview", "a1", -time.Second)
	if _, err := s.Verify("preview", tok); !errors.Is(err, ErrExpired) {
		t.Errorf("expired verify: err = %v, want ErrExpired", err)
	}
}

func TestVerifyRejectsMalformed(t *testing.T) {
	s, _ := New("secret")
	for _, tok := range []string{"", "nodot", ".", "a.", ".b", "!!!.@@@"} {
		if _, err := s.Verify("preview", tok); err == nil {
			t.Errorf("Verify(%q) = nil error, want failure", tok)
		}
	}
}

func TestSignValidatesInput(t *testing.T) {
	s, _ := New("secret")
	for _, tc := range []struct{ purpose, subject string }{
		{"", "a"}, {"p", ""}, {"pur.pose", "a"}, {"p", "sub.ject"},
	} {
		if _, err := s.Sign(tc.purpose, tc.subject, time.Hour); err == nil {
			t.Errorf("Sign(%q,%q) = nil error, want failure", tc.purpose, tc.subject)
		}
	}
}

func TestNewRejectsEmptySecret(t *testing.T) {
	if _, err := New("   "); err == nil {
		t.Fatal("expected New to reject an empty secret")
	}
}
