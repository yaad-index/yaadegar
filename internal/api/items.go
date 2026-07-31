package api

import (
	"context"
	"errors"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// itemState derives a single item's availability and reserved quantity from its
// aggregates (both owner-visible; neither carries identity).
func (s *Server) itemState(ctx context.Context, ts storage.TenantStore, it storage.Item) (storage.Availability, int, error) {
	reserved, err := ts.Items().ReservedQuantity(ctx, it.ID)
	if err != nil {
		return "", 0, err
	}
	funded, err := ts.Items().FundedAmount(ctx, it.ID)
	if err != nil {
		return "", 0, err
	}
	return deriveAvailability(it.QuantityWanted, reserved, funded), reserved, nil
}

func (s *Server) CreateItem(ctx context.Context, req gen.CreateItemRequestObject) (gen.CreateItemResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	if req.Body == nil {
		return gen.CreateItem400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("missing request body"),
		}, nil
	}
	// The item's list must exist within this tenant.
	if _, err := ts.Lists().Get(ctx, req.ListId); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.CreateItem404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("list not found"),
			}, nil
		}
		return nil, err
	}
	owned, err := s.ownsList(ctx, ts, req.ListId)
	if err != nil {
		return nil, err
	}
	if !owned {
		return gen.CreateItem403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden("not an owner of this list"),
		}, nil
	}

	created, err := ts.Items().Create(ctx, storage.Item{
		ListID:           req.ListId,
		Name:             req.Body.Name,
		URL:              req.Body.Url,
		ImageURL:         req.Body.ImageUrl,
		Price:            fromGenMoney(req.Body.Price),
		Note:             req.Body.Note,
		Priority:         derefOr(req.Body.Priority, 0),
		QuantityWanted:   derefOr(req.Body.QuantityWanted, 1),
		AllowCobuy:       req.Body.AllowCobuy,       // *bool: nil (absent) = inherit the list default (#100)
		ThankYouTemplate: req.Body.ThankYouTemplate, // *string: nil (absent) = inherit the list default (#22)
	})
	if err != nil {
		return nil, err
	}
	avail, reserved, err := s.itemState(ctx, ts, created)
	if err != nil {
		return nil, err
	}
	return gen.CreateItem201JSONResponse(toGenItem(created, avail, reserved)), nil
}

func (s *Server) ListItems(ctx context.Context, req gen.ListItemsRequestObject) (gen.ListItemsResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	if _, err := ts.Lists().Get(ctx, req.ListId); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.ListItems404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("list not found"),
			}, nil
		}
		return nil, err
	}
	owned, err := s.ownsList(ctx, ts, req.ListId)
	if err != nil {
		return nil, err
	}
	if !owned {
		return gen.ListItems403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden("not an owner of this list"),
		}, nil
	}

	page := pageParams(req.Params.Limit, req.Params.Offset)
	items, total, err := ts.Items().ListByList(ctx, req.ListId, page)
	if err != nil {
		return nil, err
	}
	// Batch the aggregates for the whole list — no per-item (N+1) queries.
	reserved, funded, err := s.listAggregates(ctx, ts, req.ListId)
	if err != nil {
		return nil, err
	}

	out := make([]gen.Item, 0, len(items))
	for _, it := range items {
		avail := deriveAvailability(it.QuantityWanted, reserved[it.ID], funded[it.ID])
		out = append(out, toGenItem(it, avail, reserved[it.ID]))
	}
	return gen.ListItems200JSONResponse(gen.ItemPage{
		Items:  &out,
		Total:  ptr(total),
		Limit:  ptr(page.Limit),
		Offset: ptr(page.Offset),
	}), nil
}

func (s *Server) UpdateItem(ctx context.Context, req gen.UpdateItemRequestObject) (gen.UpdateItemResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	if req.Body == nil {
		return nil, errors.New("missing request body")
	}
	it, err := ts.Items().Get(ctx, req.ItemId)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.UpdateItem404ApplicationProblemPlusJSONResponse{
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
		return gen.UpdateItem403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden("not an owner of this list"),
		}, nil
	}

	if req.Body.Name != nil {
		it.Name = *req.Body.Name
	}
	if req.Body.Url != nil {
		it.URL = req.Body.Url
	}
	if req.Body.ImageUrl != nil {
		it.ImageURL = req.Body.ImageUrl
	}
	if req.Body.Price != nil {
		it.Price = fromGenMoney(req.Body.Price)
	}
	if req.Body.Note != nil {
		it.Note = req.Body.Note
	}
	if req.Body.Priority != nil {
		it.Priority = *req.Body.Priority
	}
	if req.Body.QuantityWanted != nil {
		it.QuantityWanted = *req.Body.QuantityWanted
	}
	// Three-state merge-patch (#111): absent leaves the override unchanged, explicit
	// null clears it back to inheriting the list default, a value sets it on/off.
	if req.Body.AllowCobuy.IsSpecified() {
		if req.Body.AllowCobuy.IsNull() {
			it.AllowCobuy = nil
		} else {
			it.AllowCobuy = ptr(req.Body.AllowCobuy.MustGet())
		}
	}
	// Three-state thank-you override (#22/#111): absent leaves it, null clears back
	// to inheriting the list default, a value (incl. "") sets it ("" = per-item opt-out).
	if req.Body.ThankYouTemplate.IsSpecified() {
		if req.Body.ThankYouTemplate.IsNull() {
			it.ThankYouTemplate = nil
		} else {
			it.ThankYouTemplate = ptr(req.Body.ThankYouTemplate.MustGet())
		}
	}

	updated, err := ts.Items().Update(ctx, it)
	if err != nil {
		return nil, err
	}
	avail, reserved, err := s.itemState(ctx, ts, updated)
	if err != nil {
		return nil, err
	}
	return gen.UpdateItem200JSONResponse(toGenItem(updated, avail, reserved)), nil
}

func (s *Server) DeleteItem(ctx context.Context, req gen.DeleteItemRequestObject) (gen.DeleteItemResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	// Resolve the item (404) so ownership can be checked against its list before
	// deleting; a non-owner of an existing item gets 403.
	it, err := ts.Items().Get(ctx, req.ItemId)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.DeleteItem404ApplicationProblemPlusJSONResponse{
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
		return gen.DeleteItem403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden("not an owner of this list"),
		}, nil
	}
	if err := ts.Items().Delete(ctx, req.ItemId); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.DeleteItem404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("item not found"),
			}, nil
		}
		return nil, err
	}
	return gen.DeleteItem204Response{}, nil
}

// listAggregates fetches the reserved-quantity and funded-amount maps for a list
// in two grouped queries (no N+1).
func (s *Server) listAggregates(ctx context.Context, ts storage.TenantStore, listID string) (map[string]int, map[string]storage.Money, error) {
	reserved, err := ts.Items().ReservedQuantitiesByList(ctx, listID)
	if err != nil {
		return nil, nil, err
	}
	funded, err := ts.Items().FundedAmountsByList(ctx, listID)
	if err != nil {
		return nil, nil, err
	}
	return reserved, funded, nil
}
