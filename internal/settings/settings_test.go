package settings_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
