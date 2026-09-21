package api

import (
	"context"
	"errors"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/email"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// ArchiveItem marks an item finished (#419).
//
// The order matters and is not arbitrary. Warnings are computed BEFORE the write,
// so the response describes the state the owner was acting on rather than the one
// their own action produced; the giver notification is sent only when this call is
// the one that archived the item, so a repeated POST is a no-op rather than a
// second round of emails to every reserver.
func (s *Server) ArchiveItem(ctx context.Context, req gen.ArchiveItemRequestObject) (gen.ArchiveItemResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	it, err := ts.Items().Get(ctx, req.ItemId)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.ArchiveItem404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("item not found"),
			}, nil
		}
		return nil, err
	}
	owned, err := s.ownsList(ctx, ts, it.ListID)
	if err != nil {
		return nil, err
	}
	if !owned {
		return gen.ArchiveItem403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden("not an owner of this list"),
		}, nil
	}

	warnings, err := s.archiveWarnings(ctx, ts, it.ID)
	if err != nil {
		return nil, err
	}

	archivedNow, err := ts.Items().Archive(ctx, it.ID, s.clock.Now())
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.ArchiveItem404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("item not found"),
			}, nil
		}
		return nil, err
	}
	if archivedNow {
		s.notifyReserversArchived(ctx, ts, it)
	}

	updated, err := ts.Items().Get(ctx, it.ID)
	if err != nil {
		return nil, err
	}
	avail, reserved, err := s.itemState(ctx, ts, updated)
	if err != nil {
		return nil, err
	}
	item := toGenItem(updated, avail, reserved)
	return gen.ArchiveItem200JSONResponse(gen.ItemArchiveResult{
		Item:     &item,
		Warnings: &warnings,
	}), nil
}

// UnarchiveItem returns an archived item to the list. No giver is emailed: nothing
// about their reservation changed when the item was archived — it was held, not
// released — so there is nothing to tell them now.
func (s *Server) UnarchiveItem(ctx context.Context, req gen.UnarchiveItemRequestObject) (gen.UnarchiveItemResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	it, err := ts.Items().Get(ctx, req.ItemId)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.UnarchiveItem404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("item not found"),
			}, nil
		}
		return nil, err
	}
	owned, err := s.ownsList(ctx, ts, it.ListID)
	if err != nil {
		return nil, err
	}
	if !owned {
		return gen.UnarchiveItem403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden("not an owner of this list"),
		}, nil
	}
	if _, err := ts.Items().Unarchive(ctx, it.ID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.UnarchiveItem404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("item not found"),
			}, nil
		}
		return nil, err
	}
	updated, err := ts.Items().Get(ctx, it.ID)
	if err != nil {
		return nil, err
	}
	avail, reserved, err := s.itemState(ctx, ts, updated)
	if err != nil {
		return nil, err
	}
	return gen.UnarchiveItem200JSONResponse(toGenItem(updated, avail, reserved)), nil
}

// archiveWarnings reports what archiving this item affects that the owner cannot
// see for themselves. It never blocks: the maintainer's decision on #419 Q2 is
// warn, not block, because the list belongs to its owner and blocking would stop
// them acting on their own item over giver-side state they have no view of.
//
// Today the only warning is an in-flight co-buy handshake. "Mid-handshake" is
// deliberately the proposed state alone: once a match is both_confirmed the
// givers have agreed and the reveal has gone out, so archiving is the natural
// next step rather than an interruption, and warning there would train an owner
// to dismiss the warning that matters.
func (s *Server) archiveWarnings(ctx context.Context, ts storage.TenantStore, itemID string) ([]gen.ArchiveWarning, error) {
	matches, err := ts.Matches().ListByItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	warnings := make([]gen.ArchiveWarning, 0, 1)
	for _, m := range matches {
		if m.State != storage.MatchProposed {
			continue
		}
		warnings = append(warnings, gen.ArchiveWarning{
			Code: ptr(gen.CobuyMatchPending),
			Detail: ptr("Givers are part-way through arranging a co-buy for this item. " +
				"Archiving it will not undo their pledges, but they will not be able to finish here."),
		})
		break // one warning per cause, not one per row
	}
	return warnings, nil
}

// notifyReserversArchived tells every giver holding a live reservation that the
// item has come off the list (#419 Q1: the giver is told, it does not go quiet).
//
// The decision it implements: staying silent leaves a giver holding a reservation
// for something that has vanished, with no way to discover it. Telling them does
// reveal that the owner acted on the item — but the giver already knows they
// reserved it, so what is disclosed is narrow, and the alternative fails them
// outright.
//
// Each giver is emailed separately and the message names only the item, so no
// giver learns that any other giver exists (ADR-0002 §5). Best-effort, like the
// thank-you note: a send failure is logged and swallowed, because the archive
// itself has already happened and un-doing it over an SMTP error would be worse
// than a missed email.
func (s *Server) notifyReserversArchived(ctx context.Context, ts storage.TenantStore, item storage.Item) {
	reservations, err := ts.Reservations().ListByItem(ctx, item.ID)
	if err != nil {
		s.logger.ErrorContext(ctx, "archive: reservation load failed (ignored)", "item_id", item.ID, "error", err)
		return
	}
	for _, r := range reservations {
		if r.State == storage.StateExpired {
			continue // not a live hold; nobody is waiting on this item
		}
		if r.GiverEmail == nil || *r.GiverEmail == "" {
			continue
		}
		subject := "A gift you reserved has come off the list"
		body := "The owner has marked " + item.Name + " as finished, so it is no longer on their list. " +
			"There is nothing you need to do — your reservation has not been cancelled, and you will not be " +
			"asked about it again."
		if item.Name == "" {
			subject = "A gift you reserved has come off the list"
			body = "The owner has marked an item you reserved as finished, so it is no longer on their list. " +
				"There is nothing you need to do — your reservation has not been cancelled, and you will not be " +
				"asked about it again."
		}
		if err := s.email.Send(ctx, email.Message{To: *r.GiverEmail, Subject: subject, Body: body}); err != nil {
			s.logger.ErrorContext(ctx, "archive notification email failed (ignored)",
				"item_id", item.ID, "reservation_id", r.ID, "error", err)
		}
	}
}
