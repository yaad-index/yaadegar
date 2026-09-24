// Package settings holds the project-wide convention for resolving a configurable
// list setting: a list's own value if it set one, otherwise the instance default.
// Every per-list knob (decay period, response window, reserver-identity, etc.)
// resolves through Resolve so they all share one path.
package settings

import "time"

// Resolve returns the list's override when set (non-nil), otherwise the instance
// default. A nil override means "inherit the instance default".
func Resolve[T any](override *T, instanceDefault T) T {
	if override != nil {
		return *override
	}
	return instanceDefault
}

// ResolveMinutes resolves an optional per-list override stored as whole minutes
// against an instance default already expressed as a duration. It is Resolve with
// the unit conversion on the override side, which is where every minutes-valued
// knob in this project keeps its override.
//
// It exists so that a window is resolved in exactly one place no matter who needs
// it. A duration that is both ENFORCED by one component and DISPLAYED by another
// is only trustworthy if both compute it identically; two correct-looking copies
// of the same three lines can drift apart silently, and the symptom would be a
// deadline shown to someone that is not the deadline applied to them.
//
// An override of 0 resolves to 0 and wins over the default, because zero is a
// meaningful setting for these knobs (it disables the behaviour) rather than an
// absent one — Resolve keys on nil, never on the zero value.
func ResolveMinutes(overrideMinutes *int, instanceDefault time.Duration) time.Duration {
	var override *time.Duration
	if overrideMinutes != nil {
		d := time.Duration(*overrideMinutes) * time.Minute
		override = &d
	}
	return Resolve(override, instanceDefault)
}

// DisplayLayout is how an absolute instant is written for a person: a wall-clock
// time with the zone named. The zone is not decoration — a reader elsewhere needs
// to know what the time is relative to rather than assuming it is their own clock,
// which is the failure this replaces (#438).
const DisplayLayout = "2006-01-02 15:04 MST"

// FormatInstant renders t as wall-clock time in loc.
//
// Here rather than at a call site for the same reason as ResolveMinutes above: the
// confirm deadline is rendered BOTH into the email the giver acts from and onto
// the page they reserved on, and #430 established a giver may well have both in
// front of them. Two renderings of one instant invite the question of which is the
// real one, and two formatters drift in ways neither author sees — Go's zone
// abbreviation and a browser's Intl output disagree on the same zone, for example.
// So the instant is rendered once, server-side, and every surface shows that.
//
// A nil location means UTC, which is also the default an instance that configures
// nothing keeps — no deployment shifts its times silently by upgrading.
func FormatInstant(t time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	return t.In(loc).Format(DisplayLayout)
}

// ParseLocation resolves an instance's configured timezone name.
//
// An empty name is UTC, preserving the behaviour of an instance that sets nothing.
// An unknown name is an error rather than a silent fall back to UTC: a deployment
// that meant to show local time and quietly kept showing UTC would look exactly
// like one that meant UTC, and the times it mails out would be wrong with nothing
// anywhere saying so.
func ParseLocation(name string) (*time.Location, error) {
	if name == "" {
		return time.UTC, nil
	}
	return time.LoadLocation(name)
}
