package api

import (
	"context"
	"net"
	"time"
)

// Resolver looks up DNS TXT records for custom-domain verification. It is
// injected so tests use a fake with no real DNS (ADR-0004 §2).
type Resolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

// txtLookupTimeout bounds a verification DNS lookup.
const txtLookupTimeout = 5 * time.Second

// netResolver is the production Resolver backed by the system resolver, with a
// per-lookup timeout.
type netResolver struct{ r *net.Resolver }

func newNetResolver() *netResolver { return &netResolver{r: &net.Resolver{}} }

func (n *netResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, txtLookupTimeout)
	defer cancel()
	return n.r.LookupTXT(ctx, name)
}
