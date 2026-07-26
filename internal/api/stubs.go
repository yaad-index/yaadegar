package api

import (
	"context"

	"github.com/yaad-index/yaadegar/internal/api/gen"
)

// Operations outside the scope of this build return errNotImplemented, which the
// response error handler renders as 501. These land in later issues:
//   - reservations, co-buying contributions, matches → #6/#7
//   - custom domains → later owner-surface work
//   - item URL previews (server-side scrape) → later
//
// They are stubbed rather than omitted because StrictServerInterface requires
// every operation to be implemented.

func (s *Server) ListDomains(context.Context, gen.ListDomainsRequestObject) (gen.ListDomainsResponseObject, error) {
	return nil, errNotImplemented
}

func (s *Server) AddDomain(context.Context, gen.AddDomainRequestObject) (gen.AddDomainResponseObject, error) {
	return nil, errNotImplemented
}

func (s *Server) PreviewItem(context.Context, gen.PreviewItemRequestObject) (gen.PreviewItemResponseObject, error) {
	return nil, errNotImplemented
}

func (s *Server) CreateReservation(context.Context, gen.CreateReservationRequestObject) (gen.CreateReservationResponseObject, error) {
	return nil, errNotImplemented
}

func (s *Server) ReleaseReservation(context.Context, gen.ReleaseReservationRequestObject) (gen.ReleaseReservationResponseObject, error) {
	return nil, errNotImplemented
}

func (s *Server) CreateContribution(context.Context, gen.CreateContributionRequestObject) (gen.CreateContributionResponseObject, error) {
	return nil, errNotImplemented
}

func (s *Server) GetContribution(context.Context, gen.GetContributionRequestObject) (gen.GetContributionResponseObject, error) {
	return nil, errNotImplemented
}

func (s *Server) WithdrawContribution(context.Context, gen.WithdrawContributionRequestObject) (gen.WithdrawContributionResponseObject, error) {
	return nil, errNotImplemented
}

func (s *Server) ConfirmMatch(context.Context, gen.ConfirmMatchRequestObject) (gen.ConfirmMatchResponseObject, error) {
	return nil, errNotImplemented
}
