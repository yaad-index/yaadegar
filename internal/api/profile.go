package api

import (
	"context"
	"strings"

	"github.com/yaad-index/yaadegar/internal/api/gen"
)

// maxDisplayNameLen bounds a self-set display name, matching the request schema's
// maxLength so an over-long value is refused with a clear reason rather than
// truncated or pushed at the database.
const maxDisplayNameLen = 200

// UpdateProfile updates the signed-in account's own editable profile — currently
// just the display name (#185). The name defaulted to the email at creation and
// there was no self-serve edit path (admin user-update is role/ban only). A blank
// name clears the custom value and falls back to the account email, matching the
// creation default. The updated user is returned so the caller reflects the change.
func (s *Server) UpdateProfile(ctx context.Context, req gen.UpdateProfileRequestObject) (gen.UpdateProfileResponseObject, error) {
	owner, ok := ownerFromContext(ctx)
	ts, tenant, ok2 := s.tenantStore(ctx)
	if !ok || !ok2 {
		return nil, errMissingContext
	}
	if req.Body == nil {
		return gen.UpdateProfile400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("name is required"),
		}, nil
	}

	name := strings.TrimSpace(req.Body.Name)
	if len(name) > maxDisplayNameLen {
		return gen.UpdateProfile400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("the display name is too long"),
		}, nil
	}
	// Blank falls back to the account email, the same value creation seeds, so the
	// display name is never empty.
	if name == "" {
		name = owner.Email
	}

	if err := ts.Users().SetName(ctx, owner.ID, name); err != nil {
		return nil, err
	}
	updated, err := ts.Users().Get(ctx, owner.ID)
	if err != nil {
		return nil, err
	}
	return gen.UpdateProfile200JSONResponse(toGenUser(updated, tenant)), nil
}
