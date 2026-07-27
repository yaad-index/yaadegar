// Package tenant holds tenant-provisioning policy that is not persistence: the
// subdomain format + reserved-name rules applied when a tenant is created
// (ADR-0004 §5). The reserved denylist is instance config with a default set, so
// operators can extend it.
package tenant

import (
	"errors"
	"regexp"
	"strings"
)

// ErrInvalidSubdomain is returned when a subdomain is empty, malformed, or
// reserved.
var ErrInvalidSubdomain = errors.New("tenant: invalid or reserved subdomain")

// DefaultReservedSubdomains is the shipped reserved set; operators extend it via
// instance config.
var DefaultReservedSubdomains = []string{"www", "api", "app", "admin"}

// subdomainRE is a conservative DNS-label slug: lowercase alphanumeric and
// hyphens, no leading/trailing hyphen, 1–63 chars.
var subdomainRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// ValidateSubdomain checks a subdomain is set, slug-formatted, and not reserved.
// reserved is the operator's denylist (case-insensitive); pass
// DefaultReservedSubdomains for the shipped set.
func ValidateSubdomain(sub string, reserved []string) error {
	sub = strings.ToLower(strings.TrimSpace(sub))
	if !subdomainRE.MatchString(sub) {
		return ErrInvalidSubdomain
	}
	for _, r := range reserved {
		if sub == strings.ToLower(strings.TrimSpace(r)) {
			return ErrInvalidSubdomain
		}
	}
	return nil
}
