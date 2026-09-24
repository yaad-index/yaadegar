package settings_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/settings"
)

func TestResolve(t *testing.T) {
	// nil override → instance default.
	assert.Equal(t, 30, settings.Resolve(nil, 30))
	// set override (including the zero value) → the override wins.
	zero := 0
	assert.Equal(t, 0, settings.Resolve(&zero, 30))
	n := 7
	assert.Equal(t, 7, settings.Resolve(&n, 30))
}

// #438: the confirm deadline was rendered in UTC, so a giver read a wall-clock
// time that was not theirs, and the two halves of one sentence ("you have 3 days
// … until <time>") disagreed for every reader outside UTC.

func TestFormatInstantRendersWallClockInTheConfiguredZone(t *testing.T) {
	at := time.Date(2026, 9, 22, 18, 28, 0, 0, time.UTC)

	berlin, err := settings.ParseLocation("Europe/Berlin")
	require.NoError(t, err)

	assert.Equal(t, "2026-09-22 18:28 UTC", settings.FormatInstant(at, time.UTC))
	// Same instant, a different wall clock, and the zone named either way.
	assert.Equal(t, "2026-09-22 20:28 CEST", settings.FormatInstant(at, berlin))
}

func TestFormatInstantAlwaysNamesTheZone(t *testing.T) {
	// A giver reading from elsewhere must be able to see what the time is relative
	// to. A bare "20:28" is the failure mode this replaces, not a smaller version
	// of it — it is indistinguishable from their own clock.
	at := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
	for _, name := range []string{"", "UTC", "Europe/Berlin", "America/New_York", "Asia/Tehran"} {
		loc, err := settings.ParseLocation(name)
		require.NoError(t, err, name)
		got := settings.FormatInstant(at, loc)
		assert.Regexp(t, `^\d{4}-\d{2}-\d{2} \d{2}:\d{2} \S+$`, got, "zone %q", name)
	}
}

func TestFormatInstantTreatsNilAsUTC(t *testing.T) {
	at := time.Date(2026, 9, 22, 18, 28, 0, 0, time.UTC)
	assert.Equal(t, settings.FormatInstant(at, time.UTC), settings.FormatInstant(at, nil))
}

func TestParseLocationDefaultsToUTCAndRefusesTheUnknown(t *testing.T) {
	loc, err := settings.ParseLocation("")
	require.NoError(t, err)
	assert.Equal(t, time.UTC, loc)

	// Rejected rather than silently UTC: an instance that meant local time and kept
	// mailing UTC looks exactly like one that meant UTC.
	_, err = settings.ParseLocation("Mars/Olympus_Mons")
	assert.Error(t, err)
}

func TestTheSameInstantReadsDifferentlyPerZoneButNamesOneMoment(t *testing.T) {
	// The property the whole change rests on: the instant is not moved, only read.
	at := time.Date(2026, 9, 22, 18, 28, 0, 0, time.UTC)
	berlin, err := settings.ParseLocation("Europe/Berlin")
	require.NoError(t, err)

	local := settings.FormatInstant(at, berlin)
	assert.NotEqual(t, settings.FormatInstant(at, time.UTC), local,
		"a non-UTC zone must actually change the wall clock shown")

	back, err := time.ParseInLocation(settings.DisplayLayout, local, berlin)
	require.NoError(t, err)
	assert.Equal(t, at, back.UTC(), "the rendered wall clock must denote the instant it came from")
}

func TestAZoneWithNoAbbreviationIsNamedByItsOffset(t *testing.T) {
	// Worth pinning because it decides what a giver actually reads. Go's zone
	// element prints a letter abbreviation only where tzdata has one; elsewhere it
	// prints the numeric offset. Both name the zone, which is the requirement — but
	// the copy has to read acceptably in either shape, so neither is a surprise
	// later. (It also means the rendered string is not always round-trippable
	// through this layout, which is a property of Go's parser, not of the output.)
	at := time.Date(2026, 9, 22, 18, 28, 0, 0, time.UTC)

	tehran, err := settings.ParseLocation("Asia/Tehran")
	require.NoError(t, err)
	assert.Equal(t, "2026-09-22 21:58 +0330", settings.FormatInstant(at, tehran))

	berlin, err := settings.ParseLocation("Europe/Berlin")
	require.NoError(t, err)
	assert.Equal(t, "2026-09-22 20:28 CEST", settings.FormatInstant(at, berlin))
}
