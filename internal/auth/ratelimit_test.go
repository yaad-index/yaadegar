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
}
