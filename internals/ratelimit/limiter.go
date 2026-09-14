// Package ratelimit is a small in-process, per-key sliding-window limiter.
// It has no external dependency (no Redis) - fine for a single-process app
// like SADE, where the limiter only has to survive one process's lifetime.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter allows up to Limit calls per key within Window, sliding. A Limiter
// with Limit <= 0 allows everything (the zero value is "disabled"), so
// wiring code can build one straight from config without a branch.
type Limiter struct {
	limit  int
	window time.Duration

	mu   sync.Mutex
	hits map[string][]time.Time
	// sweptAt is when the map was last swept for keys with no hits left in
	// the window - without this, a key that's hit once and never again (a
	// one-off visitor's IP, say) would keep its slice forever.
	sweptAt time.Time
}

// New builds a Limiter. limit <= 0 means "no limit" - Allow always reports
// true and never allocates.
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window, hits: make(map[string][]time.Time)}
}

// Allow reports whether key may proceed right now, and if so records this
// attempt against it. Safe for concurrent use.
func (l *Limiter) Allow(key string) bool {
	if l == nil || l.limit <= 0 {
		return true
	}
	now := time.Now()
	cutoff := now.Add(-l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	kept := keepAfter(l.hits[key], cutoff)
	if len(kept) >= l.limit {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)

	l.sweep(cutoff)
	return true
}

// keepAfter returns the times strictly after cutoff, reusing hits' backing
// array (hits is only ever read by the caller holding l.mu).
func keepAfter(hits []time.Time, cutoff time.Time) []time.Time {
	out := hits[:0]
	for _, t := range hits {
		if t.After(cutoff) {
			out = append(out, t)
		}
	}
	return out
}

// sweep drops every key with no hits left in the window, at most once per
// window - called with l.mu already held.
func (l *Limiter) sweep(cutoff time.Time) {
	if time.Since(l.sweptAt) < l.window {
		return
	}
	l.sweptAt = time.Now()
	for k, hits := range l.hits {
		if len(keepAfter(hits, cutoff)) == 0 {
			delete(l.hits, k)
		}
	}
}
