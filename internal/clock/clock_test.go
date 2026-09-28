package clock_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/yaad-index/yaadegar/internal/clock"
)

func TestDistinctNeverRepeatsAndFollowsTheClockItWraps(t *testing.T) {
	t0 := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	fake := clock.NewFake(t0)
	d := clock.NewDistinct(fake)

	// Standing still: each read is one nanosecond after the last.
	assert.Equal(t, t0, d.Now())
	assert.Equal(t, t0.Add(1), d.Now())
	assert.Equal(t, t0.Add(2), d.Now())

	// Moved: reads the wrapped clock exactly again, not the accumulated offset.
	fake.Advance(time.Hour)
	assert.Equal(t, t0.Add(time.Hour), d.Now())

	// Moved backwards: follows it back rather than refusing to go earlier.
	fake.Set(t0)
	assert.Equal(t, t0, d.Now())
	assert.Equal(t, t0.Add(1), d.Now())
}
