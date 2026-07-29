package api

import (
	"context"
	"errors"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
	"github.com/yaad-index/yaadegar/internal/token"
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
	if listDisabled(list, s.clock.Now()) {
		return gen.CreateReservation410ApplicationProblemPlusJSONResponse(
			problemDetail(410, "this list is no longer active"),
		), nil
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

	raw, hash, err := token.New()
	if err != nil {
		return nil, err
	}
	// The capacity check and insert are atomic (closes the reserve oversell race):
	// a reservation past quantity_wanted returns ErrCapacityExceeded → 409.
	res, err := ts.Reservations().CreateWithinCapacity(ctx, storage.Reservation{
		ItemID:     item.ID,
		GiverName:  giverName,
		GiverEmail: giverEmail,
		Quantity:   qty,
		TokenHash:  hash,
		State:      storage.StateActive,
	}, item.QuantityWanted)
	if err != nil {
		if errors.Is(err, storage.ErrCapacityExceeded) {
			return gen.CreateReservation409ApplicationProblemPlusJSONResponse(
				problemDetail(409, "the item is already fully reserved"),
			), nil
		}
		if errors.Is(err, storage.ErrNotFound) {
			return gen.CreateReservation404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("item not found"),
			}, nil
		}
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

	res, err := ts.Reservations().ByTokenHash(ctx, token.Hash(raw))
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

// ReleaseByDecayToken releases a stale reservation via the one-click decay-release
// token emailed to the reserver. The token is single-use and stops working once
// the reservation is released or has auto-expired.
func (s *Server) ReleaseByDecayToken(ctx context.Context, req gen.ReleaseByDecayTokenRequestObject) (gen.ReleaseByDecayTokenResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	if req.Body == nil || req.Body.Token == "" {
		return gen.ReleaseByDecayToken404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound("invalid release token"),
		}, nil
	}
	res, err := ts.Reservations().ByDecayReleaseTokenHash(ctx, token.Hash(req.Body.Token))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.ReleaseByDecayToken404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("invalid release token"),
			}, nil
		}
		return nil, err
	}
	if res.State == storage.StateExpired {
		return gen.ReleaseByDecayToken410ApplicationProblemPlusJSONResponse(
			problemDetail(410, "this reservation has already expired"),
		), nil
	}
	if err := ts.Reservations().Delete(ctx, res.ID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.ReleaseByDecayToken404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("reservation not found"),
			}, nil
		}
		return nil, err
	}
	return gen.ReleaseByDecayToken204Response{}, nil
}

// KeepByDecayToken renews a stale reservation via the one-click decay-keep token:
// it resets the decay clock and invalidates both one-click tokens. Single-use;
// 410 once the reservation has auto-expired.
func (s *Server) KeepByDecayToken(ctx context.Context, req gen.KeepByDecayTokenRequestObject) (gen.KeepByDecayTokenResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	if req.Body == nil || req.Body.Token == "" {
		return gen.KeepByDecayToken404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound("invalid keep token"),
		}, nil
	}
	res, err := ts.Reservations().ByDecayKeepTokenHash(ctx, token.Hash(req.Body.Token))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.KeepByDecayToken404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("invalid keep token"),
			}, nil
		}
		return nil, err
	}
	if res.State == storage.StateExpired {
		return gen.KeepByDecayToken410ApplicationProblemPlusJSONResponse(
			problemDetail(410, "this reservation has already expired"),
		), nil
	}
	moved, err := ts.Reservations().Renew(ctx, res.ID, s.clock.Now())
	if err != nil {
		return nil, err
	}
	if !moved {
		// State changed under us (already renewed or expired) — the link is spent.
		return gen.KeepByDecayToken410ApplicationProblemPlusJSONResponse(
			problemDetail(410, "this link is no longer valid"),
		), nil
	}
	return gen.KeepByDecayToken204Response{}, nil
}
