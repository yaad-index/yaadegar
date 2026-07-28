package auth

import (
	"sync"
	"time"

	"github.com/yaad-index/yaadegar/internal/clock"
)

// Limiter throttles repeated failures by an arbitrary key (a client IP or a login
// identity). It is the seam for brute-force protection on the login endpoints.
//
// The shipped implementation (InMemoryLimiter) keeps counters in process memory,
// which is correct for a single instance. A multi-instance deployment would need a
// shared backing store (e.g. Redis) behind this same interface — that is
// deliberately not built here; swapping the implementation is the whole change.
type Limiter interface {
	// Allow reports whether an attempt for key is currently permitted (i.e. the key
	// is not in a lockout window).
	Allow(key string) bool
	// RecordFailure notes a failed attempt for key.
	RecordFailure(key string)
	// RecordSuccess clears any accumulated failures for key.
	RecordSuccess(key string)
}

// InMemoryLimiter is a per-key fixed-window failure limiter: once a key accumulates
// maxFailures failures within a window, further attempts are denied until the
// window elapses. A success clears the key. Safe for concurrent use.
type InMemoryLimiter struct {
	maxFailures int
	window      time.Duration
	clock       clock.Clock

	mu      sync.Mutex
	entries map[string]*failureWindow
}

type failureWindow struct {
	failures  int
	windowEnd time.Time
}

// NewInMemoryLimiter builds a limiter allowing up to maxFailures failures per
// window per key. A non-positive maxFailures disables limiting (Allow is always
// true). clk defaults to the real clock.
func NewInMemoryLimiter(maxFailures int, window time.Duration, clk clock.Clock) *InMemoryLimiter {
	if clk == nil {
		clk = clock.Real{}
	}
	return &InMemoryLimiter{
		maxFailures: maxFailures,
		window:      window,
		clock:       clk,
		entries:     make(map[string]*failureWindow),
	}
}

// Allow reports whether key is under its failure limit.
func (l *InMemoryLimiter) Allow(key string) bool {
	if l.maxFailures <= 0 {
		return true // limiting disabled
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok || l.clock.Now().After(e.windowEnd) {
		return true // no active window
	}
	return e.failures < l.maxFailures
}

// RecordFailure bumps key's failure count, opening a fresh window if none is active.
func (l *InMemoryLimiter) RecordFailure(key string) {
	if l.maxFailures <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock.Now()
	e, ok := l.entries[key]
	if !ok || now.After(e.windowEnd) {
		e = &failureWindow{windowEnd: now.Add(l.window)}
		l.entries[key] = e
	}
	e.failures++
}

// RecordSuccess clears key.
func (l *InMemoryLimiter) RecordSuccess(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

// NoopLimiter never limits — the default when no limiter is configured (e.g. in
// tests that don't exercise rate limiting).
type NoopLimiter struct{}

func (NoopLimiter) Allow(string) bool    { return true }
func (NoopLimiter) RecordFailure(string) {}
func (NoopLimiter) RecordSuccess(string) {}
