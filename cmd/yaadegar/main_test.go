package main

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

// parseLevel maps the CLI's log-level values to slog levels; anything else
// (which the enum already prevents) falls back to info.
func TestParseLevel(t *testing.T) {
	assert.Equal(t, slog.LevelDebug, parseLevel("debug"))
	assert.Equal(t, slog.LevelInfo, parseLevel("info"))
	assert.Equal(t, slog.LevelWarn, parseLevel("warn"))
	assert.Equal(t, slog.LevelError, parseLevel("error"))
	assert.Equal(t, slog.LevelInfo, parseLevel("nonsense"), "unknown level → info")
}
