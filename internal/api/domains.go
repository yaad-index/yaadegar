package api

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
	"github.com/yaad-index/yaadegar/internal/token"
)

// verifyTXTPrefix is the DNS label under which owners publish the verification
// token: _yaadegar-verify.<hostname>.
const verifyTXTPrefix = "_yaadegar-verify."

// hostnameRE is a conservative custom-domain check: dot-separated alphanumeric/
// hyphen labels ending in a letter TLD.
var hostnameRE = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$`)

func toGenDomain(d storage.Domain) gen.Domain {
	return gen.Domain{
		Id:                ptr(d.ID),
		Hostname:          ptr(d.Hostname),
		CnameTarget:       ptr(d.CNAMETarget),
		Verified:          ptr(d.Verified),
		TlsStatus:         ptr(gen.DomainTlsStatus(d.TLSStatus)),
		VerificationToken: ptr(d.VerificationToken),
	}
}

func (s *Server) ListDomains(ctx context.Context, _ gen.ListDomainsRequestObject) (gen.ListDomainsResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	domains, err := ts.Domains().List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]gen.Domain, 0, len(domains))
	for _, d := range domains {
		out = append(out, toGenDomain(d))
	}
	return gen.ListDomains200JSONResponse(out), nil
}

func (s *Server) AddDomain(ctx context.Context, req gen.AddDomainRequestObject) (gen.AddDomainResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	hostname := strings.ToLower(strings.TrimSpace(deref(req.Body).Hostname))
	if !hostnameRE.MatchString(hostname) {
		return gen.AddDomain400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("invalid hostname"),
		}, nil
	}
	// A stable proof-of-control challenge (not a secret capability — ADR-0004 §2).
	tok, _, err := token.New()
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	// An unverified claim older than the TTL is reclaimable, so this add can take over
	// a hostname a squatter parked but never verified (ADR-0004 §4). Zero TTL leaves
	// expiredBefore zero, which matches no claim (reclaiming disabled).
	var expiredBefore time.Time
	if s.domainClaimTTL > 0 {
		expiredBefore = now.Add(-s.domainClaimTTL)
	}
	d, err := ts.Domains().CreateReclaimingExpired(ctx, storage.Domain{
		Hostname:          hostname,
		CNAMETarget:       s.domainCNAMETarget,
		VerificationToken: tok,
		CreatedAt:         now,
	}, expiredBefore)
	if err != nil {
		if errors.Is(err, storage.ErrConflict) {
			return gen.AddDomain409ApplicationProblemPlusJSONResponse{
				ConflictApplicationProblemPlusJSONResponse: conflict("that hostname is already registered"),
			}, nil
		}
		return nil, err
	}
	return gen.AddDomain201JSONResponse(toGenDomain(d)), nil
}

func (s *Server) DeleteDomain(ctx context.Context, req gen.DeleteDomainRequestObject) (gen.DeleteDomainResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	if err := ts.Domains().Delete(ctx, req.DomainId); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.DeleteDomain404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("domain not found"),
			}, nil
		}
		return nil, err
	}
	return gen.DeleteDomain204Response{}, nil
}

// VerifyDomain checks the DNS TXT record and marks the domain verified on a
// match. Idempotent: an already-verified domain returns as-is, and a missing or
// non-matching record (incl. NXDOMAIN or a lookup timeout) returns the domain
// still unverified — a normal retry state, not an error.
func (s *Server) VerifyDomain(ctx context.Context, req gen.VerifyDomainRequestObject) (gen.VerifyDomainResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	d, err := ts.Domains().Get(ctx, req.DomainId)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.VerifyDomain404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("domain not found"),
			}, nil
		}
		return nil, err
	}
	if d.Verified {
		return gen.VerifyDomain200JSONResponse(toGenDomain(d)), nil
	}

	// A DNS error (NXDOMAIN, timeout, no record) is a not-yet-verified result, not
	// a server error — the owner retries after publishing the record.
	txts, lookupErr := s.resolver.LookupTXT(ctx, verifyTXTPrefix+d.Hostname)
	if lookupErr == nil && containsToken(txts, d.VerificationToken) {
		d.Verified = true
		updated, uerr := ts.Domains().Update(ctx, d)
		if uerr != nil {
			return nil, uerr
		}
		d = updated
	}
	return gen.VerifyDomain200JSONResponse(toGenDomain(d)), nil
}

func containsToken(txts []string, token string) bool {
	for _, t := range txts {
		if strings.TrimSpace(t) == token {
			return true
		}
	}
	return false
}

func deref(b *gen.AddDomainJSONRequestBody) gen.AddDomainJSONRequestBody {
	if b == nil {
		return gen.AddDomainJSONRequestBody{}
	}
	return *b
}
