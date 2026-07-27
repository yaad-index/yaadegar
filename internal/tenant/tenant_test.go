package tenant_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yaad-index/yaadegar/internal/tenant"
)

func TestValidateSubdomain(t *testing.T) {
	reserved := tenant.DefaultReservedSubdomains

	valid := []string{"alice", "bob-smith", "a", "team42", "x-y-z"}
	for _, s := range valid {
		assert.NoError(t, tenant.ValidateSubdomain(s, reserved), s)
	}

	// "Alice" is valid — subdomains are normalized to lowercase.
	assert.NoError(t, tenant.ValidateSubdomain("Alice", reserved))

	invalid := []string{
		"",                           // empty
		"-alice",                     // leading hyphen
		"alice-",                     // trailing hyphen
		"has space",                  // space
		"under_score",                // underscore
		"a.b",                        // dot
		"www", "api", "app", "admin", // reserved
		"WWW", // reserved, case-insensitive
	}
	for _, s := range invalid {
		assert.Error(t, tenant.ValidateSubdomain(s, reserved), s)
	}

	// Operators can extend the denylist.
	assert.Error(t, tenant.ValidateSubdomain("blog", []string{"blog"}))
}
