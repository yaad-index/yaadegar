package api

import (
	"context"
	"errors"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// CreateReservation claims units of an item on the anonymous giver surface and
// returns a one-time capability token (its unreserve handle). The giver name/
// email, if given, are stored server-side only and never surface on any response
// (ADR-0002 §5).
func (s *Server) CreateReservation(ctx context.Context, req gen.CreateReservationRequestObject) (gen.CreateReservationResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}

	list, err := ts.Lists().GetBySlug(ctx, req.ShareSlug)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.CreateReservation404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("list not found"),
			}, nil
		}
		return nil, err
	}
	if !list.Active {
		return gen.CreateReservation404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound("list not found"),
		}, nil
	}

	item, err := ts.Items().Get(ctx, req.ItemId)
	if err != nil || item.ListID != list.ID {
		if err == nil || errors.Is(err, storage.ErrNotFound) {
			return gen.CreateReservation404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("item not found"),
			}, nil
		}
		return nil, err
	}

	qty := 1
	var giverName, giverEmail *string
	if req.Body != nil {
		if req.Body.Quantity != nil && *req.Body.Quantity > 0 {
			qty = *req.Body.Quantity
		}
		giverName = req.Body.GiverName
		if req.Body.GiverEmail != nil {
			e := string(*req.Body.GiverEmail)
			giverEmail = &e
		}
	}

	// A reservation may not push the claimed total past the wanted quantity.
	reserved, err := ts.Items().ReservedQuantity(ctx, item.ID)
	if err != nil {
		return nil, err
	}
	if reserved+qty > item.QuantityWanted {
		return gen.CreateReservation409ApplicationProblemPlusJSONResponse(
			problemDetail(409, "the item is already fully reserved"),
		), nil
	}

	raw, hash, err := newCapabilityToken()
	if err != nil {
		return nil, err
	}
	res, err := ts.Reservations().Create(ctx, storage.Reservation{
		ItemID:     item.ID,
		GiverName:  giverName,
		GiverEmail: giverEmail,
		Quantity:   qty,
		TokenHash:  hash,
		DecayState: storage.DecayActive,
	})
	if err != nil {
		return nil, err
	}
	return gen.CreateReservation201JSONResponse(gen.ReservationCreated{
		ReservationId:   ptr(res.ID),
		CapabilityToken: ptr(raw),
	}), nil
}

// ReleaseReservation releases a reservation the giver holds the token for. The
// token (X-Capability-Token) is hashed and matched to the stored hash, and must
// belong to the reservation named in the path.
func (s *Server) ReleaseReservation(ctx context.Context, req gen.ReleaseReservationRequestObject) (gen.ReleaseReservationResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	raw := capTokenFromContext(ctx)
	if raw == "" {
		return gen.ReleaseReservation401ApplicationProblemPlusJSONResponse{
			UnauthorizedApplicationProblemPlusJSONResponse: unauthorized("missing capability token"),
		}, nil
	}

	res, err := ts.Reservations().ByTokenHash(ctx, hashToken(raw))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.ReleaseReservation401ApplicationProblemPlusJSONResponse{
				UnauthorizedApplicationProblemPlusJSONResponse: unauthorized("invalid capability token"),
			}, nil
		}
		return nil, err
	}
	// The token authenticates one specific reservation; it can't release another.
	if res.ID != req.ReservationId {
		return gen.ReleaseReservation404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound("reservation not found"),
		}, nil
	}

	if err := ts.Reservations().Delete(ctx, res.ID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.ReleaseReservation404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("reservation not found"),
			}, nil
		}
		return nil, err
	}
	return gen.ReleaseReservation204Response{}, nil
}
