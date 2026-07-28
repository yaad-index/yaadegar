package auth_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/clock"
)

func TestInMemoryLimiter(t *testing.T) {
	clk := clock.NewFake(epoch)
	l := auth.NewInMemoryLimiter(3, time.Minute, clk)

	// Under the limit while accumulating failures.
	for i := 0; i < 3; i++ {
		require.True(t, l.Allow("k"), "attempt %d should be allowed", i)
		l.RecordFailure("k")
	}
	// The 3rd failure trips the limit.
	assert.False(t, l.Allow("k"), "should be blocked after maxFailures")

	// A different key is unaffected.
	assert.True(t, l.Allow("other"))

	// The window expires → allowed again.
	clk.Advance(time.Minute + time.Second)
	assert.True(t, l.Allow("k"), "block lifts after the window")

	// A success clears the counter immediately.
	l.RecordFailure("k")
	l.RecordFailure("k")
	l.RecordFailure("k")
	assert.False(t, l.Allow("k"))
	l.RecordSuccess("k")
	assert.True(t, l.Allow("k"), "success resets the key")
}

func TestInMemoryLimiterDisabled(t *testing.T) {
	l := auth.NewInMemoryLimiter(0, time.Minute, clock.NewFake(epoch)) // 0 disables
	for i := 0; i < 100; i++ {
		l.RecordFailure("k")
	}
	assert.True(t, l.Allow("k"), "maxFailures<=0 disables limiting")
	assert.Zero(t, l.Len(), "a disabled limiter tracks no entries")
}

// TestInMemoryLimiterEviction: expired windows are swept so a flood of distinct
// keys can't grow the map unbounded over time, while a still-active window created
// in the same sweep survives (#65).
func TestInMemoryLimiterEviction(t *testing.T) {
	clk := clock.NewFake(epoch)
	l := auth.NewInMemoryLimiter(3, time.Minute, clk)

	l.RecordFailure("a")
	l.RecordFailure("b")
	require.Equal(t, 2, l.Len())

	// Past a+b's window (60s) and the sweep interval; a fresh failure ("c")
	// triggers the sweep, which drops the two expired keys and keeps the active one.
	clk.Advance(90 * time.Second)
	l.RecordFailure("c")
	assert.Equal(t, 1, l.Len(), "expired a,b evicted; active c survives the sweep")
	assert.True(t, l.Allow("a"), "an evicted key starts fresh")

	// c stays tracked within its window and keeps accumulating toward the limit.
	clk.Advance(30 * time.Second)
	l.RecordFailure("c")
	l.RecordFailure("c") // c now at the limit
	assert.False(t, l.Allow("c"), "active key kept its accumulated count")
}
