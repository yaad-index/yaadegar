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
// time with the zone named AND its offset from UTC. The zone is not decoration — a
// reader elsewhere needs to know what the time is relative to rather than assuming
// it is their own clock, which is the failure this replaces (#438).
//
// The offset is not redundant with the abbreviation, because abbreviations are not
// unique: CST is Central US, China and Cuba; IST is India, Ireland and Israel. An
// instance in any of those would tell a reader elsewhere "confirm by 15:04 IST",
// and a reader under a different IST would act on the wrong instant and lose the
// item at the deadline — narrower than the hard-coded UTC this replaces, which was
// inconvenient but never ambiguous (raised in review on #450). The offset makes the
// rendered string self-describing, so it no longer depends on the reader resolving
// an abbreviation the same way the instance did.
//
// The offset verb is -07:00 rather than Z07:00 deliberately: Z07:00 prints a bare
// "Z" at zero offset, so the default an unconfigured instance keeps would read
// "18:28 UTC (UTCZ)". Measured, not assumed.
const DisplayLayout = "2006-01-02 15:04 MST (UTC-07:00)"

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
// The unset test is locationUnset below and it is SHARED with LocationSource on
// purpose: widening or narrowing it here alone makes the reporter describe an
// instance the resolver disagrees with, which logs a confident false statement
// rather than a missing one (#453). Change locationUnset, never this branch.
//
// An empty name is UTC, preserving the behaviour of an instance that sets nothing.
// An unknown name is an error rather than a silent fall back to UTC: a deployment
// that meant to show local time and quietly kept showing UTC would look exactly
// like one that meant UTC, and the times it mails out would be wrong with nothing
// anywhere saying so.
//
// Zone data comes from the RUNTIME IMAGE, not the binary: this package does not
// import time/tzdata, so LoadLocation reads /usr/share/zoneinfo. The pinned
// distroless/static-debian12 base carries it (1308 entries at the digest in the
// Dockerfile, checked during review of #450). A base-image bump that dropped
// zoneinfo would turn a --timezone that had always worked into a startup failure
// with its cause invisible, so the dependency is named here rather than implied.
func ParseLocation(name string) (*time.Location, error) {
	if locationUnset(name) {
		return time.UTC, nil
	}
	return time.LoadLocation(name)
}

// locationUnset is the single test for "this instance configured no timezone".
//
// ParseLocation and LocationSource both branch on it, and they are the one pair
// where disagreeing produces a CONFIDENT FALSE statement rather than a missing
// one: a resolver treating a value as unset while the reporter calls it
// configured logs "timezone=UTC source=config" for an instance that set nothing
// usable. Adjacency does not prevent that, so the predicate is shared rather
// than written twice (#453).
func locationUnset(name string) bool {
	return name == ""
}

// LocationSource names where a resolved display zone came from: "config" when the
// instance set a name, "default" when it set nothing.
//
// This exists because the two cases produce the SAME zone. ParseLocation's empty
// branch is a deliberate compatibility default, so an instance showing UTC may
// have chosen UTC or may have configured nothing, and nothing observable told
// those apart (#453). Callers log this alongside the zone so an operator reads the
// state instead of deducing it from the process environment.
func LocationSource(name string) string {
	if locationUnset(name) {
		return "default"
	}
	return "config"
}
