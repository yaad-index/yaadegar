package api

import (
	"context"
	"errors"
	"net/mail"

	"github.com/google/uuid"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/email"
	"github.com/yaad-index/yaadegar/internal/settings"
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

	// The reserver tier resolves per-list-override → instance default (ADR-0007).
	// email_confirmed holds the item provisionally and requires an email confirm;
	// every other tier (full_guest today; registered is deferred) reserves
	// immediately as before.
	tier := settings.Resolve(list.ReserverTier, s.defaultReserverTier)
	if tier == storage.TierEmailConfirmed {
		return s.reserveEmailConfirmed(ctx, ts, item, qty, giverName, giverEmail)
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
		ReservationId:   res.ID,
		Status:          gen.ReservationCreatedStatusActive,
		CapabilityToken: ptr(raw),
	}), nil
}

// reserveEmailConfirmed handles a reserve on an email_confirmed list: it holds the
// item provisionally as pending_confirmation (via the same atomic capacity path,
// so the slot is taken immediately and counts as reserved), mints a one-time
// confirmation token, emails the giver a confirm link, and returns 202 with no
// capability token — the token is issued only once the giver confirms. The giver
// never learns any other reserver's identity, and the response carries no reserver
// contact (ADR-0002 §5, ADR-0007 §3).
func (s *Server) reserveEmailConfirmed(ctx context.Context, ts storage.TenantStore, item storage.Item, qty int, giverName, giverEmail *string) (gen.CreateReservationResponseObject, error) {
	// A confirmable reservation needs a deliverable address; a light
	// well-formedness check keeps an obviously-bad address from taking a slot only
	// to expire unconfirmed (#45's CAPTCHA seam also guards this pre-confirm path).
	if giverEmail == nil || *giverEmail == "" {
		return gen.CreateReservation400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("an email address is required to reserve on this list"),
		}, nil
	}
	if _, err := mail.ParseAddress(*giverEmail); err != nil {
		return gen.CreateReservation400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("that email address is not valid"),
		}, nil
	}
	if err := s.captchaGate(ctx); err != nil {
		return nil, err
	}

	confirmRaw, confirmHash, err := token.New()
	if err != nil {
		return nil, err
	}
	// Pre-generate the row id so the NOT NULL + UNIQUE(tenant_id, token_hash)
	// column carries a per-row-unique `pending:<id>` sentinel until the real
	// capability token is minted at confirm. The sentinel can never equal a hashed
	// capability token (64 hex), so the release path cannot match a pending row.
	resID := uuid.NewString()
	res, err := ts.Reservations().CreateWithinCapacity(ctx, storage.Reservation{
		ID:               resID,
		ItemID:           item.ID,
		GiverName:        giverName,
		GiverEmail:       giverEmail,
		Quantity:         qty,
		TokenHash:        pendingTokenHash(resID),
		ConfirmTokenHash: confirmHash,
		State:            storage.StatePendingConfirmation,
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

	// Email the confirm link. Unlike a decay reminder (which nudges an already
	// active reservation, so a failed send just retries next sweep), a pending hold
	// is useless without its confirm link — the giver can never confirm it, and it
	// would occupy the slot until the confirm window expires. So on a send failure
	// we roll the hold back (delete frees the slot immediately, sentinel and all)
	// and return 503 so the giver can retry now rather than wait out the window.
	link := s.publicLinkBase + "/confirm?token=" + confirmRaw
	if err := s.email.Send(ctx, email.Message{
		To:      *giverEmail,
		Subject: "Confirm your reservation",
		Body:    "Confirm your reservation for " + item.Name + ": " + link,
	}); err != nil {
		s.logger.ErrorContext(ctx, "confirm email send failed; rolling back the pending hold",
			"reservation_id", res.ID, "error", err)
		if derr := ts.Reservations().Delete(ctx, res.ID); derr != nil {
			// The hold is stranded but will still auto-expire at the confirm window;
			// surface a 500 so the failure is not mistaken for a clean rollback.
			s.logger.ErrorContext(ctx, "rollback of unconfirmable pending hold failed",
				"reservation_id", res.ID, "error", derr)
			return nil, derr
		}
		return gen.CreateReservation503ApplicationProblemPlusJSONResponse(
			problemDetail(503, "could not send the confirmation email; please try reserving again"),
		), nil
	}

	return gen.CreateReservation202JSONResponse(gen.ReservationCreated{
		ReservationId: res.ID,
		Status:        gen.ReservationCreatedStatusPendingConfirmation,
	}), nil
}

// ConfirmReservation activates a pending_confirmation reservation from the one-time
// token emailed to the giver, and returns the capability token to release it later.
// Idempotent: re-confirming an already-active reservation returns 200 with status
// active and no new token (it cannot be re-issued). An elapsed confirm window
// (the sweeper moved it to expired) is 410; an unknown token is 404.
func (s *Server) ConfirmReservation(ctx context.Context, req gen.ConfirmReservationRequestObject) (gen.ConfirmReservationResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	if req.Body == nil || req.Body.Token == "" {
		return gen.ConfirmReservation404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound("invalid confirmation token"),
		}, nil
	}

	res, err := ts.Reservations().ByConfirmTokenHash(ctx, token.Hash(req.Body.Token))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.ConfirmReservation404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("invalid confirmation token"),
			}, nil
		}
		return nil, err
	}

	switch res.State {
	case storage.StateExpired:
		return gen.ConfirmReservation410ApplicationProblemPlusJSONResponse(
			problemDetail(410, "this reservation's confirmation window has passed"),
		), nil
	case storage.StatePendingConfirmation:
		// First confirm: mint the capability token and install it as we transition.
		raw, hash, err := token.New()
		if err != nil {
			return nil, err
		}
		moved, err := ts.Reservations().ConfirmReservation(ctx, res.ID, hash, s.clock.Now())
		if err != nil {
			return nil, err
		}
		if moved {
			return gen.ConfirmReservation200JSONResponse(gen.ReservationConfirmed{
				ReservationId:   res.ID,
				Status:          gen.ReservationConfirmedStatusActive,
				CapabilityToken: ptr(raw),
			}), nil
		}
		// Lost a race (a concurrent confirm won, or the window expired between our
		// read and the locked transition): re-read to report the true outcome. Our
		// minted token was never stored, so we never return it.
		return s.confirmRaced(ctx, ts, res.ID)
	default:
		// Already active (idempotent re-confirm): the item is held; report active
		// with no token (the stored hash cannot be reversed to re-issue it).
		return gen.ConfirmReservation200JSONResponse(gen.ReservationConfirmed{
			ReservationId: res.ID,
			Status:        gen.ReservationConfirmedStatusActive,
		}), nil
	}
}

// confirmRaced resolves a confirm that lost the row-lock race: active → benign 200
// (no token), anything else (expired) → 410.
func (s *Server) confirmRaced(ctx context.Context, ts storage.TenantStore, id string) (gen.ConfirmReservationResponseObject, error) {
	res, err := ts.Reservations().Get(ctx, id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.ConfirmReservation404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("invalid confirmation token"),
			}, nil
		}
		return nil, err
	}
	if res.State == storage.StateActive {
		return gen.ConfirmReservation200JSONResponse(gen.ReservationConfirmed{
			ReservationId: res.ID,
			Status:        gen.ReservationConfirmedStatusActive,
		}), nil
	}
	return gen.ConfirmReservation410ApplicationProblemPlusJSONResponse(
		problemDetail(410, "this reservation's confirmation window has passed"),
	), nil
}

// pendingTokenHash is the per-row-unique sentinel stored in token_hash for a
// pending_confirmation reservation, before its capability token exists. The
// `pending:` prefix guarantees it never collides with a real hashed token, so the
// capability-release path can never match a still-pending reservation.
func pendingTokenHash(reservationID string) string { return "pending:" + reservationID }

// captchaGate is the seam for the #45 low-trust pre-confirm CAPTCHA. It is a
// deliberate no-op today (no CAPTCHA is built); the email_confirmed reserve path
// calls it so the check has a single, obvious place to land later.
func (s *Server) captchaGate(_ context.Context) error { return nil }

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
