package api

import (
	"context"
	"strings"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/preview"
)

// PreviewItem scrapes a best-effort item draft from a product URL (owner-authed).
// It never creates an item — the client reviews the draft and posts it to /items.
// Any fetch/parse failure or empty result is a 422 so the client falls back to a
// manual form. The fetch itself is SSRF-guarded in internal/preview.
func (s *Server) PreviewItem(ctx context.Context, req gen.PreviewItemRequestObject) (gen.PreviewItemResponseObject, error) {
	// Owner-only: previewing a URL is the add-item helper (only owners add items) and
	// it drives a server-side fetch — keep that capability off the giver surface
	// (ADR-0012 cut 1a role gate). Checked before anything else so a giver never
	// reaches the fetcher.
	if !hasOwnerRole(ctx) {
		return gen.PreviewItem403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(ownerRoleRequiredDetail),
		}, nil
	}
	if req.Body == nil || strings.TrimSpace(req.Body.Url) == "" {
		return previewUnfetchable(), nil
	}
	draft, err := s.previewer.Preview(ctx, req.Body.Url)
	if err != nil {
		return previewUnfetchable(), nil
	}
	return gen.PreviewItem200JSONResponse(toGenItemDraft(draft)), nil
}

func previewUnfetchable() gen.PreviewItem422ApplicationProblemPlusJSONResponse {
	return gen.PreviewItem422ApplicationProblemPlusJSONResponse(
		problemDetail(422, "could not fetch a preview for that URL; enter the item manually"),
	)
}

func toGenItemDraft(d preview.Draft) gen.ItemDraft {
	return gen.ItemDraft{
		Name:     d.Name,
		Url:      d.URL,
		ImageUrl: d.ImageURL,
		Price:    toGenMoney(d.Price),
	}
}
