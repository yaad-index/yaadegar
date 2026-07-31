package oauthlogin

import (
	"sync"
	"time"

	"github.com/yaad-index/yaadegar/internal/clock"
)

// TicketGuard enforces the one-time property of a handoff ticket: a ticket's jti
// may be consumed at most once. Signature + expiry make a ticket unforgeable and
// short-lived; this closes the remaining window in which a valid, unexpired
// ticket could be replayed (ADR-0008 §3).
type TicketGuard interface {
	// Consume records jti as used until exp, returning true only for the first
	// call; every later call for the same jti returns false. exp lets the guard
	// forget the jti once it can no longer be presented.
	Consume(jti string, exp time.Time) bool
}

// InMemoryTicketGuard is a process-local consumed-jti set. It is exactly-once
// within one instance; a multi-instance deployment would swap in a shared-state
// guard behind this interface (the same shape as the login limiter). Memory is
// bounded: entries are evicted once expired, and a ticket's TTL is seconds.
type InMemoryTicketGuard struct {
	mu    sync.Mutex
	used  map[string]time.Time // jti -> expiry
	clock clock.Clock
}

// NewInMemoryTicketGuard builds an empty guard. clk defaults to the real clock.
func NewInMemoryTicketGuard(clk clock.Clock) *InMemoryTicketGuard {
	if clk == nil {
		clk = clock.Real{}
	}
	return &InMemoryTicketGuard{used: map[string]time.Time{}, clock: clk}
}

// Consume is the single-winner check. It first evicts expired jtis (bounding
// memory to the set of still-presentable tickets), then records-or-rejects.
func (g *InMemoryTicketGuard) Consume(jti string, exp time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.clock.Now()
	for k, e := range g.used {
		if !e.After(now) {
			delete(g.used, k)
		}
	}
	if _, seen := g.used[jti]; seen {
		return false
	}
	g.used[jti] = exp
	return true
}
