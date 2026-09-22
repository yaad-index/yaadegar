package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #437: the shared email layout renders an action only for an absolute web URL,
// so a relative or empty link base produces a confirm mail with nothing to click.
// This is the check that turns that into a boot failure the operator sees rather
// than a silent one only givers experience.

func TestTheDeprecatedAliasStillStartsAnInstance(t *testing.T) {
	// The upgrade trap this exists to avoid: an instance configured entirely
	// through the older --decay-link-base is correctly set up, and must not be
	// refused at boot for having followed the older documentation.
	got, err := resolveLinkBase("", "https://lists.example.invalid")
	require.NoError(t, err)
	assert.Equal(t, "https://lists.example.invalid", got)
}

func TestThePreferredNameWinsOverTheAlias(t *testing.T) {
	got, err := resolveLinkBase("https://new.example.invalid", "https://old.example.invalid")
	require.NoError(t, err)
	assert.Equal(t, "https://new.example.invalid", got)
}

func TestAnInstanceThatCannotProduceFollowableLinksIsRefused(t *testing.T) {
	for _, tc := range []struct{ name, public, decay string }{
		{"neither set", "", ""},
		{"relative path", "/confirm", ""},
		{"host with no scheme", "lists.example.invalid", ""},
		{"scheme-relative", "//lists.example.invalid", ""},
		{"alias also unusable", "", "lists.example.invalid"},
		{"not a web scheme", "ftp://lists.example.invalid", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveLinkBase(tc.public, tc.decay)
			require.Error(t, err)
			// The message has to name the fix: the operator is the only person in
			// the chain who can act on it.
			assert.Contains(t, err.Error(), "--public-link-base")
			assert.Contains(t, err.Error(), "--decay-link-base")
		})
	}
}

func TestSchemeMatchingIsCaseInsensitive(t *testing.T) {
	// A base typed in a config file is not normalised anywhere before this.
	for _, base := range []string{"HTTPS://lists.example.invalid", "Http://lists.example.invalid"} {
		got, err := resolveLinkBase(base, "")
		require.NoError(t, err, base)
		assert.Equal(t, base, got, "the value is passed through unchanged, not lowercased")
	}
}
