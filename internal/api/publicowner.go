package api

import (
	"context"
	"errors"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// ownerPageListsCap bounds the rows one owner page returns. Personal collections are
// small — the per-list public view caps its items the same way and for the same
// reason — and a later pass can paginate if that stops being true.
const ownerPageListsCap = 200

// GetPublicOwner serves an owner's public list index by their owner key (#308): one
// durable link an owner shares once, instead of one link per list.
//
// Everything this returns is scoped by the storage read, not by what the response
// happens to render. ListedByOwner selects only lists marked listed, so an unlisted
// list's share_slug is never loaded here and cannot leak through a rendering change
// later. Lists whose own share link would answer 410 are dropped through the same
// listDisabled path that produces the 410, so the two surfaces refuse the same lists
// for the same reason rather than through two rules that can drift apart.
func (s *Server) GetPublicOwner(ctx context.Context, req gen.GetPublicOwnerRequestObject) (gen.GetPublicOwnerResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}

	owner, err := ts.Users().ByOwnerKey(ctx, req.OwnerKey)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.GetPublicOwner404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("owner page not found"),
			}, nil
		}
		return nil, err
	}

	lists, err := ts.Lists().ListedByOwner(ctx, owner.ID, storage.Page{Limit: ownerPageListsCap})
	if err != nil {
		return nil, err
	}

	// Drop the lists their own share link would refuse. A list that answers 410 is
	// not advertised on an index.
	now := s.clock.Now()
	served := make([]storage.List, 0, len(lists))
	for _, l := range lists {
		if !listDisabled(l, now) {
			served = append(served, l)
		}
	}

	// One batch read for the preview clusters, not an N+1 per row — the same shape
	// the owner's own dashboard uses.
	ids := make([]string, len(served))
	for i, l := range served {
		ids[i] = l.ID
	}
	previews, err := ts.Items().PreviewsByLists(ctx, ids, previewsPerCard)
	if err != nil {
		return nil, err
	}

	rows := make([]gen.PublicOwnerList, 0, len(served))
	for _, l := range served {
		rows = append(rows, gen.PublicOwnerList{
			Title:        ptr(l.Title),
			ShareSlug:    ptr(l.ShareSlug),
			ItemCount:    ptr(l.ItemCount),
			ItemPreviews: toGenItemPreviews(previews[l.ID]),
		})
	}

	return gen.GetPublicOwner200JSONResponse(gen.PublicOwner{
		DisplayName: publicDisplayName(owner),
		Lists:       &rows,
	}), nil
}

// publicDisplayName is the owner's name as a public page may show it, or nil when it
// must not show one.
//
// A display name defaults to the account email at creation and falls back to it
// again whenever the owner blanks the field (#185), so users.name holds a literal
// email address for anyone who never set one. Rendering it as this page's heading
// would publish that address to everyone holding the key. Returning nil instead
// leaves the page a neutral heading, which is the correct outcome for an owner who
// never chose a public name.
func publicDisplayName(owner storage.User) *string {
	if owner.Name == "" || owner.Name == owner.Email {
		return nil
	}
	return ptr(owner.Name)
}
