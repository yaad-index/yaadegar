// Package clock provides an injectable time source so time-dependent logic — the
// reservation-decay sweep and its grace/expire windows — is testable with a fake
// clock instead of real sleeps.
package clock

import (
	"sync"
	"time"
)

// Clock is a source of the current time.
type Clock interface {
	Now() time.Time
}

// Real is the wall clock (UTC).
type Real struct{}

func (Real) Now() time.Time { return time.Now().UTC() }

// Fake is a settable clock for tests. Safe for concurrent use so a background
// sweeper and the test goroutine can share it under -race.
type Fake struct {
	mu sync.Mutex
	t  time.Time
}

// NewFake returns a Fake set to t (in UTC).
func NewFake(t time.Time) *Fake { return &Fake{t: t.UTC()} }

func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

// Set moves the clock to t.
func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = t.UTC()
}

// Advance moves the clock forward by d.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = f.t.Add(d)
}
