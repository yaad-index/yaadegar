package main

import (
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The config sample (config.example.yaml) is hand-maintained documentation of a
// machine-readable surface — the CLI struct tags — and hand-maintained docs of a
// generated surface drift silently. This guard makes the drift loud: it walks the
// CLI struct tags for every YAADEGAR_ env var and fails if the sample documents a
// different set, in either direction. It is why the one switch that governs whether
// strangers can sign up cannot go undocumented again without a red build.
//
// The match is on the ENV VAR NAME, which appears in the sample for every key: the
// normal keys carry an inline "(env YAADEGAR_X, flag --y)" note, and the env-only
// secrets are listed by their env name in the secrets section. So a key documented
// in either form counts as present, and only a genuinely absent (or genuinely stale)
// env var trips the guard.

// collectEnvTags walks a struct type, recursing into nested struct fields (the CLI
// holds each subcommand as a struct field), and records every non-empty `env:` tag.
func collectEnvTags(t reflect.Type, out map[string]bool) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if env := f.Tag.Get("env"); env != "" {
			out[env] = true
		}
		collectEnvTags(f.Type, out)
	}
}

func TestConfigSampleMatchesEnvVars(t *testing.T) {
	// Every YAADEGAR_ env var the binary actually reads, taken from the struct tags
	// rather than a second hand-kept list.
	code := map[string]bool{}
	collectEnvTags(reflect.TypeOf(CLI{}), code)
	for k := range code {
		if !strings.HasPrefix(k, "YAADEGAR_") {
			delete(code, k)
		}
	}
	require.NotEmpty(t, code, "reflection found no YAADEGAR_ env tags — the walk is broken, not the sample")

	// Every YAADEGAR_ env var named anywhere in the sample.
	data, err := os.ReadFile("../../config.example.yaml")
	require.NoError(t, err)
	documented := map[string]bool{}
	for _, m := range regexp.MustCompile(`YAADEGAR_[A-Z_]+`).FindAllString(string(data), -1) {
		documented[m] = true
	}

	var missing, stale []string
	for k := range code {
		if !documented[k] {
			missing = append(missing, k)
		}
	}
	for k := range documented {
		if !code[k] {
			stale = append(stale, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)

	assert.Empty(t, missing,
		"config.example.yaml is missing env vars the binary reads — document them (add the key with its env/flag/default, or list the secret in the secrets section)")
	assert.Empty(t, stale,
		"config.example.yaml names env vars the binary no longer reads — remove them from the sample")
}
