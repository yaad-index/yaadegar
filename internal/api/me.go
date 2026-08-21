package api

import (
	"context"

	"github.com/yaad-index/yaadegar/internal/api/gen"
)

func (s *Server) GetCurrentUser(ctx context.Context, _ gen.GetCurrentUserRequestObject) (gen.GetCurrentUserResponseObject, error) {
	owner, ok := ownerFromContext(ctx)
	_, tenant, ok2 := s.tenantStore(ctx)
	if !ok || !ok2 {
		return nil, errMissingContext
	}
	return gen.GetCurrentUser200JSONResponse(toGenUser(owner, tenant)), nil
}

func (s *Server) GetHealthz(_ context.Context, _ gen.GetHealthzRequestObject) (gen.GetHealthzResponseObject, error) {
	return gen.GetHealthz200TextResponse("ok"), nil
}

// GetVersion reports the running API build version (ADR-0014 §3). Unauthenticated and
// instance-level: the tenant-resolution and owner-auth middleware skip it, so it is
// reachable through the web edge's /api/v1/ passthrough without a tenant or a token,
// which is what lets a monitoring check poll it to detect a mismatched image pair. An
// empty stamp (a server constructed without one) reports "unknown" rather than a blank,
// matching the version subcommand's fail-distinct contract (#225).
func (s *Server) GetVersion(_ context.Context, _ gen.GetVersionRequestObject) (gen.GetVersionResponseObject, error) {
	v := s.version
	if v == "" {
		v = "unknown"
	}
	return gen.GetVersion200JSONResponse(gen.VersionInfo{Version: v}), nil
}
