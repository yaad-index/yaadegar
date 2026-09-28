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

// Distinct reads c but never returns the same instant twice in a row. While c
// stands still, each further read is one nanosecond after the one before; once c
// moves (Set, Advance, or real time passing) it reads c exactly again.
//
// A test hands this to the store, not to the code under test. A Fake stamps
// every row it creates with one instant, and ordering by that timestamp then
// falls through to the id tie-break, which is random: rows come back in a
// different order from the one they were created in.
type Distinct struct {
	c    Clock
	mu   sync.Mutex
	last time.Time
	n    time.Duration
}

// NewDistinct wraps c.
func NewDistinct(c Clock) *Distinct { return &Distinct{c: c} }

func (d *Distinct) Now() time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	t := d.c.Now()
	if t.Equal(d.last) {
		d.n++
	} else {
		d.last, d.n = t, 0
	}
	return t.Add(d.n)
}
