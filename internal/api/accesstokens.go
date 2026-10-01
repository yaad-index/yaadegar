package api

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
	tok "github.com/yaad-index/yaadegar/internal/token"
)

const (
	// maxActiveAccessTokens is how many active personal access tokens an account
	// may hold (ADR-0016 §6).
	maxActiveAccessTokens = 20
	// accessTokenFreshness is how recently the acting session must have been
	// issued to create a token (ADR-0016 §5).
	accessTokenFreshness = 10 * time.Minute
	// maxAccessTokenName bounds the user-chosen token name.
	maxAccessTokenName = 100
)

// tokenRefusal is the 403 detail for a credential operation attempted with a
// personal access token (ADR-0016 §2).
const tokenRefusal = "a personal access token cannot manage tokens; sign in to do that"

// ListAccessTokens lists the account's personal access tokens (ADR-0016 §7).
func (s *Server) ListAccessTokens(ctx context.Context, _ gen.ListAccessTokensRequestObject) (gen.ListAccessTokensResponseObject, error) {
	owner, ok := ownerFromContext(ctx)
	ts, _, ok2 := s.tenantStore(ctx)
	if !ok || !ok2 {
		return nil, errMissingContext
	}
	if _, ok := viaToken(ctx); ok {
		return gen.ListAccessTokens403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse: forbidden(tokenRefusal)}, nil
	}
	list, err := ts.AccessTokens().ListByUser(ctx, owner.ID)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	out := make(gen.ListAccessTokens200JSONResponse, 0, len(list))
	for _, t := range list {
		out = append(out, accessTokenView(t, now))
	}
	return out, nil
}

// CreateAccessToken creates a personal access token and returns its value once.
// It needs a session issued within accessTokenFreshness (ADR-0016 §5); a session
// that carries no issue time is not fresh.
func (s *Server) CreateAccessToken(ctx context.Context, req gen.CreateAccessTokenRequestObject) (gen.CreateAccessTokenResponseObject, error) {
	owner, ok := ownerFromContext(ctx)
	ts, _, ok2 := s.tenantStore(ctx)
	if !ok || !ok2 {
		return nil, errMissingContext
	}
	if _, ok := viaToken(ctx); ok {
		return gen.CreateAccessToken403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse: forbidden(tokenRefusal)}, nil
	}
	now := s.clock.Now()
	issued := sessionIssued(ctx)
	if issued.IsZero() || now.Sub(issued) > accessTokenFreshness {
		return gen.CreateAccessToken403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden("creating a token needs a recent sign-in; sign in again and retry"),
		}, nil
	}
	if req.Body == nil {
		return gen.CreateAccessToken400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse: badRequest("a request body is required")}, nil
	}
	name := strings.TrimSpace(req.Body.Name)
	if name == "" || len([]rune(name)) > maxAccessTokenName {
		return gen.CreateAccessToken400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse: badRequest("name must be 1 to 100 characters")}, nil
	}
	never := req.Body.NeverExpires != nil && *req.Body.NeverExpires
	switch {
	case never && req.Body.ExpiresAt != nil:
		return gen.CreateAccessToken400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse: badRequest("give expires_at or never_expires, not both")}, nil
	case !never && req.Body.ExpiresAt == nil:
		return gen.CreateAccessToken400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse: badRequest("choose an expiry, or set never_expires to true")}, nil
	case !never && !req.Body.ExpiresAt.After(now):
		return gen.CreateAccessToken400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse: badRequest("expires_at must be in the future")}, nil
	}

	raw, hash, err := tok.NewPAT()
	if err != nil {
		return nil, err
	}
	t, err := ts.AccessTokens().Create(ctx, storage.AccessToken{
		UserID: owner.ID, Name: name, TokenHash: hash, Last4: raw[len(raw)-4:], ExpiresAt: req.Body.ExpiresAt,
	}, maxActiveAccessTokens, now)
	if errors.Is(err, storage.ErrTooManyTokens) {
		return gen.CreateAccessToken409ApplicationProblemPlusJSONResponse{
			ConflictApplicationProblemPlusJSONResponse: conflict("this account already has 20 active tokens; revoke one first"),
		}, nil
	}
	if err != nil {
		return nil, err
	}
	s.logger.Info("personal access token created", "user_id", owner.ID, "token_id", t.ID, "name", t.Name)
	return gen.CreateAccessToken201JSONResponse{Token: raw, AccessToken: accessTokenView(t, now)}, nil
}

// RevokeAccessToken revokes one of the account's personal access tokens.
func (s *Server) RevokeAccessToken(ctx context.Context, req gen.RevokeAccessTokenRequestObject) (gen.RevokeAccessTokenResponseObject, error) {
	owner, ok := ownerFromContext(ctx)
	ts, _, ok2 := s.tenantStore(ctx)
	if !ok || !ok2 {
		return nil, errMissingContext
	}
	if _, ok := viaToken(ctx); ok {
		return gen.RevokeAccessToken403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse: forbidden(tokenRefusal)}, nil
	}
	revoked, err := ts.AccessTokens().Revoke(ctx, owner.ID, req.TokenId, s.clock.Now())
	if errors.Is(err, storage.ErrNotFound) {
		return gen.RevokeAccessToken404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: notFound("no such token")}, nil
	}
	if err != nil {
		return nil, err
	}
	if revoked {
		name := ""
		if list, err := ts.AccessTokens().ListByUser(ctx, owner.ID); err == nil {
			for _, t := range list {
				if t.ID == req.TokenId {
					name = t.Name
				}
			}
		}
		s.logger.Info("personal access token revoked", "user_id", owner.ID, "token_id", req.TokenId, "name", name)
	}
	return gen.RevokeAccessToken204Response{}, nil
}

// accessTokenView is a token as the API shows it: never its value or hash.
func accessTokenView(t storage.AccessToken, now time.Time) gen.AccessToken {
	return gen.AccessToken{
		Id: t.ID, Name: t.Name, Last4: t.Last4, CreatedAt: t.CreatedAt,
		ExpiresAt: t.ExpiresAt, LastUsedAt: t.LastUsedAt, RevokedAt: t.RevokedAt,
		Active: t.ActiveAt(now),
	}
}

// activeTokenCount counts the user's personal access tokens that work at now.
func activeTokenCount(ctx context.Context, ts storage.TenantStore, userID string, now time.Time) (int, error) {
	list, err := ts.AccessTokens().ListByUser(ctx, userID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, t := range list {
		if t.ActiveAt(now) {
			n++
		}
	}
	return n, nil
}
