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

	assert.Equal(t, "2026-09-22 18:28 UTC (UTC+00:00)", settings.FormatInstant(at, time.UTC))
	// Same instant, a different wall clock, and the zone named either way.
	assert.Equal(t, "2026-09-22 20:28 CEST (UTC+02:00)", settings.FormatInstant(at, berlin))
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
		assert.Regexp(t, `^\d{4}-\d{2}-\d{2} \d{2}:\d{2} \S+ \(UTC[+-]\d{2}:\d{2}\)$`, got, "zone %q", name)
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

func TestTwoZonesSharingAnAbbreviationStillReadDifferently(t *testing.T) {
	// The finding this layout exists for (#450 review): an abbreviation does not
	// identify a zone. Kolkata and Dublin both print IST, so on the abbreviation
	// alone a giver under one cannot tell which instant the other meant — and
	// acting on the wrong one loses the item at the deadline. The offset is what
	// makes each string denote a single moment.
	//
	// A positive control, not a 0-stays-0 test: both strings MUST contain IST, so
	// the ambiguity is really present and the test cannot pass vacuously.
	//
	// The offset assertions are the ones that BIND, and the NotEqual below does
	// not. Verified by mutation (layout reverted to "MST" alone): NotEqual still
	// passed, because the two zones also differ by wall clock, and only the
	// (UTC+HH:MM) assertions failed. Keeping NotEqual as a statement of intent,
	// but a future reader should not read it as the guard.
	at := time.Date(2026, 9, 22, 18, 28, 0, 0, time.UTC)

	kolkata, err := settings.ParseLocation("Asia/Kolkata")
	require.NoError(t, err)
	dublin, err := settings.ParseLocation("Europe/Dublin")
	require.NoError(t, err)

	india, ireland := settings.FormatInstant(at, kolkata), settings.FormatInstant(at, dublin)
	require.Contains(t, india, "IST")
	require.Contains(t, ireland, "IST")
	assert.NotEqual(t, india, ireland,
		"two zones sharing an abbreviation must not render identically")
	assert.Contains(t, india, "(UTC+05:30)")
	assert.Contains(t, ireland, "(UTC+01:00)")
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
	assert.Equal(t, "2026-09-22 21:58 +0330 (UTC+03:30)", settings.FormatInstant(at, tehran))

	berlin, err := settings.ParseLocation("Europe/Berlin")
	require.NoError(t, err)
	assert.Equal(t, "2026-09-22 20:28 CEST (UTC+02:00)", settings.FormatInstant(at, berlin))
}

func TestTheSourceOfAResolvedZoneIsDistinguishableFromTheZoneItself(t *testing.T) {
	// The point of #453: an unset timezone and a deliberate "UTC" resolve to the
	// SAME location, so the zone alone cannot report which happened. The assertion
	// that carries the requirement is the inequality — a refactor that collapsed
	// both cases to one label would still satisfy the two Equal checks below while
	// destroying the only thing this function exists to say.
	unsetZone, err := settings.ParseLocation("")
	require.NoError(t, err)
	chosenZone, err := settings.ParseLocation("UTC")
	require.NoError(t, err)
	assert.Equal(t, unsetZone.String(), chosenZone.String(),
		"the premise: configuring nothing and choosing UTC are indistinguishable by zone")

	assert.NotEqual(t, settings.LocationSource(""), settings.LocationSource("UTC"),
		"so the source must tell them apart, or the startup line reports nothing")

	assert.Equal(t, "default", settings.LocationSource(""))
	assert.Equal(t, "config", settings.LocationSource("UTC"))
	assert.Equal(t, "config", settings.LocationSource("Europe/Berlin"))
}
