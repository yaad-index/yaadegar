package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/yaad-index/yaadegar/internal/api/gen"
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
	if !list.Active || (list.EventDate != nil && list.EventDate.Before(startOfToday())) {
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
		out = append(out, toGenPublicItem(it, avail, funded[it.ID]))
	}
	return gen.GetPublicList200JSONResponse(gen.PublicList{
		Title:     ptr(list.Title),
		EventDate: toGenDate(list.EventDate),
		Items:     &out,
	}), nil
}

// startOfToday is midnight UTC today; event dates are compared against it so a
// list stays live through its whole event day and disables the day after.
func startOfToday() time.Time {
	n := time.Now().UTC()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}
