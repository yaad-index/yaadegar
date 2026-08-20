package main

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

// resolveVersion is the pure core of what a build reports (#225). All three branches
// are exercised here — including the "unknown" fallback, which exists precisely so a
// build that can determine nothing announces itself instead of masquerading as an
// ordinary build; a branch no test reaches is a branch that is not really there.
func TestResolveVersion(t *testing.T) {
	vcs := func(revision, modified string) []debug.BuildSetting {
		return []debug.BuildSetting{
			{Key: "vcs.revision", Value: revision},
			{Key: "vcs.modified", Value: modified},
		}
	}

	t.Run("release semver, when link-stamped, wins over any VCS info", func(t *testing.T) {
		assert.Equal(t, "v0.13.0", resolveVersion("v0.13.0", vcs("0aa5a65f2fdf2e44", "false")))
	})

	t.Run("no link version falls back to the short VCS commit", func(t *testing.T) {
		assert.Equal(t, "0aa5a65f2fdf", resolveVersion("", vcs("0aa5a65f2fdf2e448a602db8a83c491339c90f39", "false")))
	})

	t.Run("a modified tree gets a -dirty suffix", func(t *testing.T) {
		assert.Equal(t, "0aa5a65f2fdf-dirty", resolveVersion("", vcs("0aa5a65f2fdf2e448a602db8a83c491339c90f39", "true")))
	})

	t.Run("no link version and no VCS revision reports unknown, not dev", func(t *testing.T) {
		assert.Equal(t, "unknown", resolveVersion("", nil))
		// VCS settings present but without a revision key (e.g. -buildvcs=false) also
		// resolve to unknown, not to an empty or partial string.
		assert.Equal(t, "unknown", resolveVersion("", []debug.BuildSetting{{Key: "GOOS", Value: "linux"}}))
	})
}
