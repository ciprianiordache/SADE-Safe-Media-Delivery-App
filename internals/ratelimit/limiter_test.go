package ratelimit

import (
	"testing"
	"time"
)

func TestAllowsUpToLimitThenBlocks(t *testing.T) {
	l := New(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !l.Allow("a") {
			t.Fatalf("call %d: Allow(a) = false, want true", i)
		}
	}
	if l.Allow("a") {
		t.Error("4th call: Allow(a) = true, want false")
	}
}

func TestKeysAreIndependent(t *testing.T) {
	l := New(1, time.Minute)
	if !l.Allow("a") {
		t.Fatal("Allow(a) = false, want true")
	}
	if !l.Allow("b") {
		t.Error("Allow(b) = false, want true - a different key must not share a's budget")
	}
	if l.Allow("a") {
		t.Error("second Allow(a) = true, want false")
	}
}

func TestWindowSlidesOpenAgain(t *testing.T) {
	l := New(1, 20*time.Millisecond)
	if !l.Allow("a") {
		t.Fatal("Allow(a) = false, want true")
	}
	if l.Allow("a") {
		t.Fatal("second Allow(a) = true, want false (still within the window)")
	}
	time.Sleep(30 * time.Millisecond)
	if !l.Allow("a") {
		t.Error("Allow(a) after the window elapsed = false, want true")
	}
}

func TestZeroLimitDisables(t *testing.T) {
	l := New(0, time.Minute)
	for i := 0; i < 100; i++ {
		if !l.Allow("a") {
			t.Fatalf("call %d: Allow(a) = false with limit 0, want always true", i)
		}
	}
}

func TestNilLimiterAllowsEverything(t *testing.T) {
	var l *Limiter
	if !l.Allow("a") {
		t.Error("nil Limiter.Allow = false, want true")
	}
}
