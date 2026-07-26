// Package settings holds the project-wide convention for resolving a configurable
// list setting: a list's own value if it set one, otherwise the instance default.
// Every per-list knob (decay period, response window, reserver-identity, etc.)
// resolves through Resolve so they all share one path.
package settings

// Resolve returns the list's override when set (non-nil), otherwise the instance
// default. A nil override means "inherit the instance default".
func Resolve[T any](override *T, instanceDefault T) T {
	if override != nil {
		return *override
	}
	return instanceDefault
}
