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
