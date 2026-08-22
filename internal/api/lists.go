package api

import (
	"context"
	"errors"
	"unicode/utf8"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// maxListDescriptionLen caps the owner-editable list description (#143). A sane
// bound on a giver-facing free-text field: long enough for a real blurb, short
// enough that it can't be abused with an oversized body. Counted in runes so the
// limit is about characters, not bytes.
const maxListDescriptionLen = 2000

// previewsPerCard is how many item thumbnails the dashboard list card previews
// before the "+N" overflow chip (#207): the delivered design shows up to three.
const previewsPerCard = 3

func (s *Server) CreateList(ctx context.Context, req gen.CreateListRequestObject) (gen.CreateListResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	owner, ok2 := ownerFromContext(ctx)
	if !ok || !ok2 {
		return nil, errMissingContext
	}
	// Owner-role gate (ADR-0009): only an owner account may author lists; a giver
	// self-registered account is refused before any body/store work.
	if !hasOwnerRole(ctx) {
		return gen.CreateList403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(ownerRoleRequiredDetail),
		}, nil
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
	// Feed the dashboard card preview cluster (#207) in one batch read, not an N+1
	// per card. Previews live on the summary only: the single-list reads (Get) have
	// no cluster, so ListRepo.List leaves them nil and we fill them here.
	ids := make([]string, len(lists))
	for i, l := range lists {
		ids[i] = l.ID
	}
	previews, err := ts.Items().PreviewsByLists(ctx, ids, previewsPerCard)
	if err != nil {
		return nil, err
	}
	out := make([]gen.List, 0, len(lists))
	for _, l := range lists {
		l.ItemPreviews = previews[l.ID]
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
	// The override fields are three-state (#111): absent leaves the value, explicit
	// null clears it (event_date → no date; the others → inherit the instance
	// default), a value sets it. Storage already treats a nil domain pointer as
	// inherit, so a clear just writes nil.
	if req.Body.EventDate.IsSpecified() {
		if req.Body.EventDate.IsNull() {
			l.EventDate = nil
		} else {
			d := req.Body.EventDate.MustGet()
			l.EventDate = fromGenDate(&d)
		}
	}
	if req.Body.DecayDays.IsSpecified() {
		if req.Body.DecayDays.IsNull() {
			l.DecayDays = nil
		} else {
			l.DecayDays = ptr(req.Body.DecayDays.MustGet())
		}
	}
	if req.Body.ReserverConfirmWindow.IsSpecified() {
		if req.Body.ReserverConfirmWindow.IsNull() {
			l.ReserverConfirmWindowMinutes = nil
		} else {
			l.ReserverConfirmWindowMinutes = ptr(req.Body.ReserverConfirmWindow.MustGet())
		}
	}
	if req.Body.ReserverTier.IsSpecified() {
		if req.Body.ReserverTier.IsNull() {
			l.ReserverTier = nil
		} else {
			s := req.Body.ReserverTier.MustGet()
			tier, ok := parseReserverTier(&s)
			if !ok {
				return gen.UpdateList400ApplicationProblemPlusJSONResponse{
					BadRequestApplicationProblemPlusJSONResponse: badRequest("invalid reserver_tier"),
				}, nil
			}
			l.ReserverTier = tier
		}
	}
	if req.Body.Active != nil {
		l.Active = *req.Body.Active
	}
	if req.Body.AllowCobuy != nil {
		l.AllowCobuy = *req.Body.AllowCobuy // list-level co-buy default (#100)
	}
	if req.Body.ThankYouTemplate != nil {
		l.ThankYouTemplate = *req.Body.ThankYouTemplate // list-level thank-you default (#22); "" = off
	}
	if req.Body.Description != nil {
		// Owner-editable list description (#143); "" = none. Cap the raw length here —
		// the markdown is rendered to sanitized HTML in the frontend load (the same
		// renderNote path item notes use, ADR-0006), never in this handler.
		if utf8.RuneCountInString(*req.Body.Description) > maxListDescriptionLen {
			return gen.UpdateList400ApplicationProblemPlusJSONResponse{
				BadRequestApplicationProblemPlusJSONResponse: badRequest("description is too long"),
			}, nil
		}
		l.Description = *req.Body.Description
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
