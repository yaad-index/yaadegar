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
