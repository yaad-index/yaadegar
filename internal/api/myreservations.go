package api

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// CreateMyReservation reserves an item as the authenticated account (ADR-0012
// Decision 4 / cut 3). The reservation is bound to the caller's account server-side
// (reserver_user_id) so it appears on the account's own dashboard and satisfies the
// `registered` reserver tier — the account is the proof of the tier. No per-
// reservation confirmation email fires (the account's email was verified at
// registration), so it is active immediately on any tier; a registered account may
// reserve on a lower-tier list too (the tier is a floor, ADR-0009 Decision 5). No
// anonymous capability token is issued — the account manages this reservation through
// its dashboard. Anonymity to the owner is unchanged: the account binding is
// server-side only and never surfaces on an owner view (ADR-0002 §5).
func (s *Server) CreateMyReservation(ctx context.Context, req gen.CreateMyReservationRequestObject) (gen.CreateMyReservationResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	user, ok := ownerFromContext(ctx)
	if !ok {
		return nil, errMissingContext
	}
	if req.Body == nil || req.Body.ShareSlug == "" || req.Body.ItemId == "" {
		return gen.CreateMyReservation400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("share_slug and item_id are required"),
		}, nil
	}

	list, err := ts.Lists().GetBySlug(ctx, req.Body.ShareSlug)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.CreateMyReservation404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("list not found"),
			}, nil
		}
		return nil, err
	}
	if listDisabled(list, s.clock.Now()) {
		return gen.CreateMyReservation410ApplicationProblemPlusJSONResponse(
			problemDetail(410, "this list is no longer active"),
		), nil
	}

	item, err := ts.Items().Get(ctx, req.Body.ItemId)
	if err != nil || item.ListID != list.ID {
		if err == nil || errors.Is(err, storage.ErrNotFound) {
			return gen.CreateMyReservation404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("item not found"),
			}, nil
		}
		return nil, err
	}

	qty := 1
	if req.Body.Quantity != nil && *req.Body.Quantity > 0 {
		qty = *req.Body.Quantity
	}

	// Pre-generate the id so token_hash carries a per-row-unique `account:<id>`
	// sentinel — this reservation has no anonymous capability token. The account's
	// email/name ride the existing (owner-invisible) giver_* fields so the thank-you
	// note and decay reminders keep working; they never surface to the owner.
	resID := uuid.NewString()
	uid := user.ID
	res := storage.Reservation{
		ID:             resID,
		ItemID:         item.ID,
		Quantity:       qty,
		TokenHash:      accountTokenHash(resID),
		State:          storage.StateActive,
		ReserverUserID: &uid,
	}
	if user.Email != "" {
		email := user.Email
		res.GiverEmail = &email
	}
	if user.Name != "" {
		name := user.Name
		res.GiverName = &name
	}

	created, err := ts.Reservations().CreateWithinCapacity(ctx, res, item.QuantityWanted)
	if err != nil {
		if errors.Is(err, storage.ErrCrossTrackConflict) {
			return gen.CreateMyReservation409ApplicationProblemPlusJSONResponse(
				problemDetail(409, "this item is being co-bought and can't be reserved"),
			), nil
		}
		if errors.Is(err, storage.ErrCapacityExceeded) {
			return gen.CreateMyReservation409ApplicationProblemPlusJSONResponse(
				problemDetail(409, "the item is already fully reserved"),
			), nil
		}
		if errors.Is(err, storage.ErrNotFound) {
			return gen.CreateMyReservation404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("item not found"),
			}, nil
		}
		return nil, err
	}
	s.sendThankYou(ctx, ts, created)
	return gen.CreateMyReservation201JSONResponse(gen.ReservationCreated{
		ReservationId: created.ID,
		Status:        gen.ReservationCreatedStatusActive,
	}), nil
}

// ListMyReservations returns the authenticated account's own reservations across
// lists — the "things I've reserved" dashboard (ADR-0012 Decision 4 / #20). It is the
// account's own view, keyed on its account; it discloses nothing to any list owner.
func (s *Server) ListMyReservations(ctx context.Context, req gen.ListMyReservationsRequestObject) (gen.ListMyReservationsResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	user, ok := ownerFromContext(ctx)
	if !ok {
		return nil, errMissingContext
	}
	rows, total, err := ts.Reservations().ListByReserver(ctx, user.ID, pageParams(req.Params.Limit, req.Params.Offset))
	if err != nil {
		return nil, err
	}
	items := make([]gen.MyReservation, 0, len(rows))
	for _, r := range rows {
		items = append(items, toGenMyReservation(r))
	}
	return gen.ListMyReservations200JSONResponse(gen.MyReservationPage{Items: items, Total: total}), nil
}

// DeleteMyReservation releases one of the authenticated account's own reservations
// (ADR-0012 Decision 4). Ownership is by the account binding; a reservation the caller
// does not own — or an anonymous one — is reported as not found, disclosing nothing.
func (s *Server) DeleteMyReservation(ctx context.Context, req gen.DeleteMyReservationRequestObject) (gen.DeleteMyReservationResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	user, ok := ownerFromContext(ctx)
	if !ok {
		return nil, errMissingContext
	}
	res, err := ts.Reservations().Get(ctx, req.ReservationId)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.DeleteMyReservation404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("reservation not found"),
			}, nil
		}
		return nil, err
	}
	if res.ReserverUserID == nil || *res.ReserverUserID != user.ID {
		return gen.DeleteMyReservation404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound("reservation not found"),
		}, nil
	}
	if err := ts.Reservations().Delete(ctx, res.ID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.DeleteMyReservation404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("reservation not found"),
			}, nil
		}
		return nil, err
	}
	return gen.DeleteMyReservation204Response{}, nil
}

// accountTokenHash is the per-row-unique sentinel stored in token_hash for an
// account-bound reservation (ADR-0012 cut 3), which has no anonymous capability token
// — it is managed through the reserver's dashboard. The `account:` prefix guarantees
// it never collides with a real hashed token (64 hex), so the capability-release path
// can never match an account-bound reservation.
func accountTokenHash(reservationID string) string { return "account:" + reservationID }

// toGenMyReservation maps a dashboard read-model row to its API view. It carries only
// the reserver's own reservation context — no other party's identity.
func toGenMyReservation(r storage.ReserverReservation) gen.MyReservation {
	return gen.MyReservation{
		ReservationId: r.ReservationID,
		ItemId:        r.ItemID,
		ItemName:      r.ItemName,
		ListTitle:     r.ListTitle,
		ShareSlug:     r.ShareSlug,
		Quantity:      r.Quantity,
		State:         gen.MyReservationState(r.State),
		CreatedAt:     r.CreatedAt,
	}
}
