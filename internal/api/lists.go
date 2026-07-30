package api

import (
	"context"
	"errors"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
)

func (s *Server) CreateList(ctx context.Context, req gen.CreateListRequestObject) (gen.CreateListResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	owner, ok2 := ownerFromContext(ctx)
	if !ok || !ok2 {
		return nil, errMissingContext
	}
	if req.Body == nil {
		return gen.CreateList400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("missing request body"),
		}, nil
	}
	tier, ok3 := parseReserverTier(req.Body.ReserverTier)
	if !ok3 {
		return gen.CreateList400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("invalid reserver_tier"),
		}, nil
	}
	created, err := ts.Lists().Create(ctx, storage.List{
		Title:                        req.Body.Title,
		Visibility:                   fromGenVisibility(req.Body.Visibility),
		EventDate:                    fromGenDate(req.Body.EventDate),
		DecayDays:                    req.Body.DecayDays,             // nil (absent) = inherit the instance default
		ReserverTier:                 tier,                           // nil (absent) = inherit the instance default
		ReserverConfirmWindowMinutes: req.Body.ReserverConfirmWindow, // nil (absent) = inherit
		Active:                       true,
	}, owner.ID)
	if err != nil {
		return nil, err
	}
	return gen.CreateList201JSONResponse(toGenList(created)), nil
}

// ownsList reports whether the request's authenticated owner owns listID — the
// owner-surface authorization check (ADR-0005 §7). It is called only by the
// authenticated owner handlers; the public/giver surface (public.go, share-slug
// reads, reserve, contribute) never gates on ownership.
func (s *Server) ownsList(ctx context.Context, ts storage.TenantStore, listID string) (bool, error) {
	owner, ok := ownerFromContext(ctx)
	if !ok {
		return false, errMissingContext
	}
	return ts.Lists().IsOwner(ctx, listID, owner.ID)
}

func (s *Server) ListLists(ctx context.Context, req gen.ListListsRequestObject) (gen.ListListsResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	owner, ok2 := ownerFromContext(ctx)
	if !ok || !ok2 {
		return nil, errMissingContext
	}
	page := pageParams(req.Params.Limit, req.Params.Offset)
	lists, total, err := ts.Lists().List(ctx, owner.ID, page)
	if err != nil {
		return nil, err
	}
	out := make([]gen.List, 0, len(lists))
	for _, l := range lists {
		out = append(out, toGenList(l))
	}
	return gen.ListLists200JSONResponse(gen.ListPage{
		Items:  &out,
		Total:  ptr(total),
		Limit:  ptr(page.Limit),
		Offset: ptr(page.Offset),
	}), nil
}

func (s *Server) GetList(ctx context.Context, req gen.GetListRequestObject) (gen.GetListResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	l, err := ts.Lists().Get(ctx, req.ListId)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.GetList404ApplicationProblemPlusJSONResponse{
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
		return gen.GetList403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden("not an owner of this list"),
		}, nil
	}
	return gen.GetList200JSONResponse(toGenList(l)), nil
}

func (s *Server) UpdateList(ctx context.Context, req gen.UpdateListRequestObject) (gen.UpdateListResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	if req.Body == nil {
		return gen.UpdateList400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("missing request body"),
		}, nil
	}
	l, err := ts.Lists().Get(ctx, req.ListId)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.UpdateList404ApplicationProblemPlusJSONResponse{
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
		return gen.UpdateList403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden("not an owner of this list"),
		}, nil
	}

	// Merge-patch: a present field is applied; an absent one is left as-is. Because
	// the spec's optional fields are omitempty pointers, event_date can be set but
	// not cleared through PATCH — a known limitation until a clear-semantics pass.
	if req.Body.Title != nil {
		l.Title = *req.Body.Title
	}
	if req.Body.Visibility != nil {
		l.Visibility = storage.Visibility(*req.Body.Visibility)
	}
	if req.Body.EventDate != nil {
		l.EventDate = fromGenDate(req.Body.EventDate)
	}
	if req.Body.DecayDays != nil {
		l.DecayDays = req.Body.DecayDays
	}
	if req.Body.ReserverConfirmWindow != nil {
		l.ReserverConfirmWindowMinutes = req.Body.ReserverConfirmWindow
	}
	if req.Body.ReserverTier != nil {
		tier, ok := parseReserverTier(req.Body.ReserverTier)
		if !ok {
			return gen.UpdateList400ApplicationProblemPlusJSONResponse{
				BadRequestApplicationProblemPlusJSONResponse: badRequest("invalid reserver_tier"),
			}, nil
		}
		l.ReserverTier = tier
	}
	if req.Body.Active != nil {
		l.Active = *req.Body.Active
	}
	if req.Body.AllowCobuy != nil {
		l.AllowCobuy = *req.Body.AllowCobuy // list-level co-buy default (#100)
	}

	updated, err := ts.Lists().Update(ctx, l)
	if err != nil {
		return nil, err
	}
	return gen.UpdateList200JSONResponse(toGenList(updated)), nil
}

func (s *Server) DeleteList(ctx context.Context, req gen.DeleteListRequestObject) (gen.DeleteListResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	// Resolve existence first (404) so a non-owner gets 403, not a leak-free 404,
	// only for a list that truly exists.
	if _, err := ts.Lists().Get(ctx, req.ListId); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.DeleteList404ApplicationProblemPlusJSONResponse{
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
		return gen.DeleteList403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden("not an owner of this list"),
		}, nil
	}
	if err := ts.Lists().Delete(ctx, req.ListId); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.DeleteList404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("list not found"),
			}, nil
		}
		return nil, err
	}
	return gen.DeleteList204Response{}, nil
}
