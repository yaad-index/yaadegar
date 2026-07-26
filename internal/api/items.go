package api

import (
	"context"
	"errors"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// itemAvailability derives a single item's availability from its aggregates.
func (s *Server) itemAvailability(ctx context.Context, ts storage.TenantStore, it storage.Item) (storage.Availability, error) {
	reserved, err := ts.Items().ReservedQuantity(ctx, it.ID)
	if err != nil {
		return "", err
	}
	funded, err := ts.Items().FundedAmount(ctx, it.ID)
	if err != nil {
		return "", err
	}
	return deriveAvailability(it.QuantityWanted, reserved, funded), nil
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
	// The item's list must exist within this tenant. The spec offers no 404 on
	// this operation, so a missing list is reported as a 400 (follow-up: add 404).
	if _, err := ts.Lists().Get(ctx, req.ListId); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.CreateItem400ApplicationProblemPlusJSONResponse{
				BadRequestApplicationProblemPlusJSONResponse: badRequest("list not found"),
			}, nil
		}
		return nil, err
	}

	created, err := ts.Items().Create(ctx, storage.Item{
		ListID:         req.ListId,
		Name:           req.Body.Name,
		URL:            req.Body.Url,
		ImageURL:       req.Body.ImageUrl,
		Price:          fromGenMoney(req.Body.Price),
		Note:           req.Body.Note,
		Priority:       derefOr(req.Body.Priority, 0),
		QuantityWanted: derefOr(req.Body.QuantityWanted, 1),
	})
	if err != nil {
		return nil, err
	}
	avail, err := s.itemAvailability(ctx, ts, created)
	if err != nil {
		return nil, err
	}
	return gen.CreateItem201JSONResponse(toGenItem(created, avail)), nil
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
		out = append(out, toGenItem(it, avail))
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

	updated, err := ts.Items().Update(ctx, it)
	if err != nil {
		return nil, err
	}
	avail, err := s.itemAvailability(ctx, ts, updated)
	if err != nil {
		return nil, err
	}
	return gen.UpdateItem200JSONResponse(toGenItem(updated, avail)), nil
}

func (s *Server) DeleteItem(ctx context.Context, req gen.DeleteItemRequestObject) (gen.DeleteItemResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
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
