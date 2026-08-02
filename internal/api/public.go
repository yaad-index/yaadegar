package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/settings"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// publicItemsCap bounds the items returned in a single public list view. Personal
// lists are small; a later pass can paginate the public surface if needed.
const publicItemsCap = 500

func (s *Server) GetPublicList(ctx context.Context, req gen.GetPublicListRequestObject) (gen.GetPublicListResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	list, err := ts.Lists().GetBySlug(ctx, req.ShareSlug)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.GetPublicList404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("list not found"),
			}, nil
		}
		return nil, err
	}

	// A disabled list, or one past its event date, is Gone — the share link no
	// longer serves it (ADR-0002: the public view returns 410 for these).
	if listDisabled(list, s.clock.Now()) {
		return gen.GetPublicList410ApplicationProblemPlusJSONResponse(
			problemDetail(http.StatusGone, "this list is no longer active"),
		), nil
	}

	items, _, err := ts.Items().ListByList(ctx, list.ID, storage.Page{Limit: publicItemsCap})
	if err != nil {
		return nil, err
	}
	reserved, funded, err := s.listAggregates(ctx, ts, list.ID)
	if err != nil {
		return nil, err
	}

	out := make([]gen.PublicItem, 0, len(items))
	for _, it := range items {
		avail := deriveAvailability(it.QuantityWanted, reserved[it.ID], funded[it.ID])
		out = append(out, toGenPublicItem(it, avail, funded[it.ID], resolveAllowCobuy(it, list)))
	}

	// email_required (#144): mirror the tier the reserve path enforces so the giver UI
	// can require the email up front instead of failing at submit. Only the
	// email-confirmed tier rejects a missing giver email (reserveEmailConfirmed); the
	// full-guest tier does not, so the flag is false there.
	tier := settings.Resolve(list.ReserverTier, s.defaultReserverTier)
	emailRequired := tier == storage.TierEmailConfirmed
	return gen.GetPublicList200JSONResponse(gen.PublicList{
		Title:         ptr(list.Title),
		Description:   ptr(list.Description),
		EventDate:     toGenDate(list.EventDate),
		EmailRequired: &emailRequired,
		Items:         &out,
	}), nil
}

// listDisabled reports whether a list is effectively disabled for the giver
// surface: explicitly inactive, or strictly past its event date. Computed lazily
// at request time (no per-row state), so it is always correct with no lag. An
// open-ended list (nil event date) never disables on time.
func listDisabled(l storage.List, now time.Time) bool {
	return !l.Active || (l.EventDate != nil && l.EventDate.Before(startOfDay(now)))
}

// startOfDay is midnight UTC of now; event dates are compared against it so a
// list stays live through its whole event day and disables the day after.
func startOfDay(now time.Time) time.Time {
	u := now.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}
