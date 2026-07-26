package api

import (
	"context"

	"github.com/yaad-index/yaadegar/internal/api/gen"
)

// Operations outside the scope of this build return errNotImplemented, which the
// response error handler renders as 501:
//   - custom domains → later owner-surface work
//
// They are stubbed rather than omitted because StrictServerInterface requires
// every operation to be implemented.

func (s *Server) ListDomains(context.Context, gen.ListDomainsRequestObject) (gen.ListDomainsResponseObject, error) {
	return nil, errNotImplemented
}

func (s *Server) AddDomain(context.Context, gen.AddDomainRequestObject) (gen.AddDomainResponseObject, error) {
	return nil, errNotImplemented
}
