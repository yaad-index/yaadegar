package api

import (
	"context"
	"errors"
	"strings"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/email"
	"github.com/yaad-index/yaadegar/internal/storage"
	"github.com/yaad-index/yaadegar/internal/token"
)

// MinMatchContributions is the named rule: co-buying is for splitting, so a match
// is only proposed when at least this many contributions cover the price. A lone
// full-price pledge stays pending and never self-matches.
const MinMatchContributions = 2

// CreateContribution records an anonymous pledge toward co-buying an item and,
// when pending pledges reach coverage with >= MinMatchContributions parties,
// proposes a Match and emails the parties. Returns a one-time capability token.
func (s *Server) CreateContribution(ctx context.Context, req gen.CreateContributionRequestObject) (gen.CreateContributionResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	if req.Body == nil {
		return gen.CreateContribution400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("missing request body"),
		}, nil
	}
	pledged := fromGenMoney(&req.Body.Pledged)
	if pledged == nil || pledged.AmountMinor <= 0 {
		return gen.CreateContribution400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("pledged amount must be positive"),
		}, nil
	}
	if string(req.Body.ContactEmail) == "" {
		return gen.CreateContribution400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("contact_email is required"),
		}, nil
	}

	list, err := ts.Lists().GetBySlug(ctx, req.ShareSlug)
	if err != nil || !list.Active {
		if err == nil || errors.Is(err, storage.ErrNotFound) {
			return contribNotFound("list not found"), nil
		}
		return nil, err
	}
	item, err := ts.Items().Get(ctx, req.ItemId)
	if err != nil || item.ListID != list.ID {
		if err == nil || errors.Is(err, storage.ErrNotFound) {
			return contribNotFound("item not found"), nil
		}
		return nil, err
	}
	if item.Price == nil {
		return gen.CreateContribution400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("this item has no price to co-buy toward"),
		}, nil
	}
	if pledged.Currency != item.Price.Currency {
		return gen.CreateContribution400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("pledge currency must match the item price currency"),
		}, nil
	}

	raw, hash, err := token.New()
	if err != nil {
		return nil, err
	}
	c, err := ts.Contributions().CreateWithinCapacity(ctx, storage.Contribution{
		ItemID:       item.ID,
		Pledged:      *pledged,
		GiverName:    req.Body.GiverName,
		ContactEmail: string(req.Body.ContactEmail),
		TokenHash:    hash,
	}, item.Price.AmountMinor)
	if err != nil {
		if errors.Is(err, storage.ErrCapacityExceeded) {
			return gen.CreateContribution409ApplicationProblemPlusJSONResponse(
				problemDetail(409, "this pledge would overfund the item"),
			), nil
		}
		if errors.Is(err, storage.ErrNotFound) {
			return contribNotFound("item not found"), nil
		}
		return nil, err
	}

	// Try to form a match now that this pledge landed. A proposal failure must not
	// lose the committed contribution, so it is logged and the contribution is
	// still returned (the match can form on a later pledge or be reconciled).
	var proposed *gen.Match
	if m, ok, err := s.maybeProposeMatch(ctx, ts, item); err != nil {
		s.logger.Error("match proposal failed", "err", err, "item", item.ID)
	} else if ok {
		gm := toGenMatch(m, nil) // proposed state → no contacts
		proposed = &gm
	}

	return gen.CreateContribution201JSONResponse(gen.ContributionCreated{
		ContributionId:  ptr(c.ID),
		CapabilityToken: ptr(raw),
		Match:           proposed,
	}), nil
}

func contribNotFound(detail string) gen.CreateContribution404ApplicationProblemPlusJSONResponse {
	return gen.CreateContribution404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(detail),
	}
}

// maybeProposeMatch proposes a match over the item's pending contributions when
// there are >= MinMatchContributions of them and they cover the price. Creating
// the match stamps those contributions matched, so the same set can't be
// proposed twice. On proposal it emails each party (best-effort, logged).
func (s *Server) maybeProposeMatch(ctx context.Context, ts storage.TenantStore, item storage.Item) (storage.Match, bool, error) {
	if item.Price == nil {
		return storage.Match{}, false, nil
	}
	all, err := ts.Contributions().ListByItem(ctx, item.ID)
	if err != nil {
		return storage.Match{}, false, err
	}
	var pending []storage.Contribution
	var sum int64
	for _, c := range all {
		if c.Status == storage.ContributionPending {
			pending = append(pending, c)
			sum += c.Pledged.AmountMinor
		}
	}
	if len(pending) < MinMatchContributions || sum < item.Price.AmountMinor {
		return storage.Match{}, false, nil
	}

	ids := make([]string, len(pending))
	for i, c := range pending {
		ids[i] = c.ID
	}
	m, err := ts.Matches().Create(ctx, storage.Match{ItemID: item.ID, ContributionIDs: ids})
	if err != nil {
		return storage.Match{}, false, err
	}
	s.emailMatch(ctx, pending, "A co-buying match is proposed",
		"A group gift you pledged toward is fully funded. Confirm or decline with your capability token.")
	return m, true, nil
}

// GetContribution returns the giver's own contribution (incl. match_id when a
// match is pending). It never carries another party's contact.
func (s *Server) GetContribution(ctx context.Context, req gen.GetContributionRequestObject) (gen.GetContributionResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	c, err := s.contributionByToken(ctx, ts, req.ContributionId)
	if err != nil {
		switch {
		case errors.Is(err, errTokenUnauthorized):
			return gen.GetContribution401ApplicationProblemPlusJSONResponse{
				UnauthorizedApplicationProblemPlusJSONResponse: unauthorized("invalid capability token"),
			}, nil
		case errors.Is(err, errContribNotFound):
			return gen.GetContribution404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("contribution not found"),
			}, nil
		default:
			return nil, err
		}
	}
	return gen.GetContribution200JSONResponse(toGenContribution(c)), nil
}

// WithdrawContribution withdraws a pending pledge. If it is part of a not-yet-
// confirmed match, the match dissolves and the other pledges return to pending.
// Withdrawing after both parties confirmed is a 409.
func (s *Server) WithdrawContribution(ctx context.Context, req gen.WithdrawContributionRequestObject) (gen.WithdrawContributionResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	c, err := s.contributionByToken(ctx, ts, req.ContributionId)
	if err != nil {
		switch {
		case errors.Is(err, errTokenUnauthorized):
			return gen.WithdrawContribution401ApplicationProblemPlusJSONResponse{
				UnauthorizedApplicationProblemPlusJSONResponse: unauthorized("invalid capability token"),
			}, nil
		case errors.Is(err, errContribNotFound):
			return gen.WithdrawContribution404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("contribution not found"),
			}, nil
		default:
			return nil, err
		}
	}

	if c.MatchID != nil {
		m, err := ts.Matches().Get(ctx, *c.MatchID)
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
		if err == nil {
			if m.State == storage.MatchBothConfirmed || m.State == storage.MatchDone {
				return gen.WithdrawContribution409ApplicationProblemPlusJSONResponse{
					ConflictApplicationProblemPlusJSONResponse: conflict("cannot withdraw after both parties confirmed"),
				}, nil
			}
			if err := s.dissolveMatch(ctx, ts, m, c.ID); err != nil {
				return nil, err
			}
		}
	}

	if err := ts.Contributions().Delete(ctx, c.ID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.WithdrawContribution404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("contribution not found"),
			}, nil
		}
		return nil, err
	}
	return gen.WithdrawContribution204Response{}, nil
}

// ConfirmMatch records one party's confirm/decline. Contacts are revealed (in the
// response and by email) only once both parties have confirmed.
func (s *Server) ConfirmMatch(ctx context.Context, req gen.ConfirmMatchRequestObject) (gen.ConfirmMatchResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	raw := capTokenFromContext(ctx)
	if raw == "" {
		return confirmUnauthorized("missing capability token"), nil
	}
	m, err := ts.Matches().Get(ctx, req.MatchId)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return confirmNotFound("match not found"), nil
		}
		return nil, err
	}
	c, err := ts.Contributions().ByTokenHash(ctx, token.Hash(raw))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return confirmUnauthorized("invalid capability token"), nil
		}
		return nil, err
	}
	if c.MatchID == nil || *c.MatchID != m.ID {
		return confirmNotFound("this token is not part of that match"), nil
	}
	if m.State != storage.MatchProposed {
		return gen.ConfirmMatch409ApplicationProblemPlusJSONResponse(
			problemDetail(409, "this match is already resolved"),
		), nil
	}

	if req.Body != nil && req.Body.Decision == gen.Decline {
		c.Status = storage.ContributionDeclined
		if _, err := ts.Contributions().Update(ctx, c); err != nil {
			return nil, err
		}
		if err := s.dissolveMatch(ctx, ts, m, c.ID); err != nil {
			return nil, err
		}
		m.State = storage.MatchDeclined
		return gen.ConfirmMatch200JSONResponse(toGenMatch(m, nil)), nil
	}

	// Confirm.
	c.Status = storage.ContributionConfirmed
	if _, err := ts.Contributions().Update(ctx, c); err != nil {
		return nil, err
	}
	contribs, allConfirmed, err := s.matchContributions(ctx, ts, m)
	if err != nil {
		return nil, err
	}
	if !allConfirmed {
		return gen.ConfirmMatch200JSONResponse(toGenMatch(m, nil)), nil
	}

	m.State = storage.MatchBothConfirmed
	if _, err := ts.Matches().Update(ctx, m); err != nil {
		return nil, err
	}
	contacts := make([]string, 0, len(contribs))
	for _, cc := range contribs {
		contacts = append(contacts, cc.ContactEmail)
	}
	s.emailReveal(ctx, contribs)
	return gen.ConfirmMatch200JSONResponse(toGenMatch(m, contacts)), nil
}

func confirmUnauthorized(d string) gen.ConfirmMatchResponseObject {
	return gen.ConfirmMatch401ApplicationProblemPlusJSONResponse{
		UnauthorizedApplicationProblemPlusJSONResponse: unauthorized(d),
	}
}

func confirmNotFound(d string) gen.ConfirmMatchResponseObject {
	return gen.ConfirmMatch404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(d),
	}
}

// Token-auth outcomes for the capability-token-secured contribution endpoints.
// Each handler maps these to its own typed 401/404 response.
var (
	errTokenUnauthorized = errors.New("api: unauthorized capability token")
	errContribNotFound   = errors.New("api: contribution not found")
)

// contributionByToken resolves and authorizes a contribution from the request's
// capability token. It returns errTokenUnauthorized (missing/invalid token),
// errContribNotFound (token valid but not this contribution), or a real error.
func (s *Server) contributionByToken(ctx context.Context, ts storage.TenantStore, contributionID string) (storage.Contribution, error) {
	raw := capTokenFromContext(ctx)
	if raw == "" {
		return storage.Contribution{}, errTokenUnauthorized
	}
	c, err := ts.Contributions().ByTokenHash(ctx, token.Hash(raw))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return storage.Contribution{}, errTokenUnauthorized
		}
		return storage.Contribution{}, err
	}
	if c.ID != contributionID {
		return storage.Contribution{}, errContribNotFound
	}
	return c, nil
}

// dissolveMatch returns the match's other contributions to pending (clearing
// their match link) and marks the match declined. The excepted contribution
// (the decliner/withdrawer) is handled by the caller.
func (s *Server) dissolveMatch(ctx context.Context, ts storage.TenantStore, m storage.Match, exceptID string) error {
	for _, id := range m.ContributionIDs {
		if id == exceptID {
			continue
		}
		c, err := ts.Contributions().Get(ctx, id)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				continue
			}
			return err
		}
		c.Status = storage.ContributionPending
		c.MatchID = nil
		if _, err := ts.Contributions().Update(ctx, c); err != nil {
			return err
		}
	}
	m.State = storage.MatchDeclined
	if _, err := ts.Matches().Update(ctx, m); err != nil {
		return err
	}
	return nil
}

// matchContributions loads the match's contributions and reports whether all are
// confirmed.
func (s *Server) matchContributions(ctx context.Context, ts storage.TenantStore, m storage.Match) ([]storage.Contribution, bool, error) {
	out := make([]storage.Contribution, 0, len(m.ContributionIDs))
	allConfirmed := true
	for _, id := range m.ContributionIDs {
		c, err := ts.Contributions().Get(ctx, id)
		if err != nil {
			return nil, false, err
		}
		if c.Status != storage.ContributionConfirmed {
			allConfirmed = false
		}
		out = append(out, c)
	}
	return out, allConfirmed, nil
}

// emailMatch sends the same non-revealing notice to each party. The body carries
// no other party's contact.
func (s *Server) emailMatch(ctx context.Context, contribs []storage.Contribution, subject, body string) {
	for _, c := range contribs {
		if err := s.email.Send(ctx, email.Message{To: c.ContactEmail, Subject: subject, Body: body}); err != nil {
			s.logger.Error("co-buying email failed", "err", err, "contribution", c.ID)
		}
	}
}

// emailReveal sends each party the other parties' contacts — only called once the
// match is both_confirmed.
func (s *Server) emailReveal(ctx context.Context, contribs []storage.Contribution) {
	for _, c := range contribs {
		var others []string
		for _, o := range contribs {
			if o.ID != c.ID {
				others = append(others, o.ContactEmail)
			}
		}
		body := "Both parties confirmed. Coordinate the gift with: " + strings.Join(others, ", ")
		if err := s.email.Send(ctx, email.Message{To: c.ContactEmail, Subject: "Your co-buying match is confirmed", Body: body}); err != nil {
			s.logger.Error("co-buying reveal email failed", "err", err, "contribution", c.ID)
		}
	}
}
