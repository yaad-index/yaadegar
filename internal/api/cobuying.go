package api

import (
	"context"
	"errors"
	"strings"
	"time"

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
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return contribNotFound("list not found"), nil
		}
		return nil, err
	}
	if listDisabled(list, s.clock.Now()) {
		return gen.CreateContribution410ApplicationProblemPlusJSONResponse(
			problemDetail(410, "this list is no longer active"),
		), nil
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
	// Owner opt-out (#100): resolved before the capacity layer so policy never
	// reaches storage. 403 (not 409) — the item is not capacity-conflicted, the
	// owner has simply disallowed co-buying, so it is reserve-only.
	if !resolveAllowCobuy(item, list) {
		return gen.CreateContribution403ApplicationProblemPlusJSONResponse(
			problemDetail(403, "co-buying is turned off for this item"),
		), nil
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
		if errors.Is(err, storage.ErrCrossTrackConflict) {
			return gen.CreateContribution409ApplicationProblemPlusJSONResponse(
				problemDetail(409, "this item is reserved and can't be co-bought"),
			), nil
		}
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

	// Mint a scoped, expiring match-action token per participant so the emailed
	// confirm/decline link works on any device (#96). A non-positive window means
	// no expiry. The raw token rides only the email link; only its hash is stored.
	var expiresAt *time.Time
	if s.cobuyConfirmWindow > 0 {
		exp := s.clock.Now().Add(s.cobuyConfirmWindow)
		expiresAt = &exp
	}
	links := make(map[string]string, len(pending))
	for _, c := range pending {
		raw, hash, err := token.New()
		if err != nil {
			return storage.Match{}, false, err
		}
		if err := ts.Contributions().SetMatchActionToken(ctx, c.ID, hash, expiresAt); err != nil {
			return storage.Match{}, false, err
		}
		links[c.ID] = s.publicLinkBase + "/cobuy/" + m.ID + "?t=" + raw
	}
	s.emailMatch(ctx, pending, links)
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
			// Only a still-proposed match is dissolved here. A match already in a
			// terminal state (declined, or expired by the auto-expiry sweep #101) has
			// released or expired its pledges; re-running dissolveMatch on it would flip
			// the terminal siblings back to pending, resurrecting them and re-blocking
			// reserve under #93. A terminal match just falls through to the delete below.
			if m.State == storage.MatchProposed {
				if err := s.dissolveMatch(ctx, ts, m, c.ID); err != nil {
					return nil, err
				}
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
	// Dual-auth: the capability token (same-browser) OR the scoped match-action
	// token (cross-device, #96). Either way the token must belong to this match.
	raw := capTokenFromContext(ctx)
	c, err := s.resolveMatchActor(ctx, ts, raw, req.MatchId)
	if err != nil {
		switch {
		case errors.Is(err, errTokenUnauthorized), errors.Is(err, errActionTokenExpired):
			return confirmUnauthorized("invalid or expired token"), nil
		case errors.Is(err, errActorNotInMatch):
			return confirmNotFound("this token is not part of that match"), nil
		}
		return nil, err
	}
	m, err := ts.Matches().Get(ctx, req.MatchId)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return confirmNotFound("match not found"), nil
		}
		return nil, err
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
		// The decliner's action token is spent; the others' are cleared as they are
		// released back to pending in dissolveMatch.
		if err := ts.Contributions().SetMatchActionToken(ctx, c.ID, "", nil); err != nil {
			return nil, err
		}
		if err := s.dissolveMatch(ctx, ts, m, c.ID); err != nil {
			return nil, err
		}
		m.State = storage.MatchDeclined
		return gen.ConfirmMatch200JSONResponse(toGenMatch(m, nil)), nil
	}

	// Confirm. The transition — mark this contribution confirmed, check whether all
	// are confirmed, and flip the match to both_confirmed — runs atomically under the
	// item lock in the store, so two concurrent confirms complete the match (and fire
	// the reveal) exactly once (#36).
	m, contribs, completedNow, err := ts.Matches().ConfirmContribution(ctx, m.ItemID, m.ID, c.ID)
	if err != nil {
		return nil, err
	}
	if !completedNow {
		return gen.ConfirmMatch200JSONResponse(toGenMatch(m, nil)), nil
	}
	contacts := make([]string, 0, len(contribs))
	for _, cc := range contribs {
		contacts = append(contacts, cc.ContactEmail)
		// The match is resolved; the scoped tokens are spent, so clear them.
		if err := ts.Contributions().SetMatchActionToken(ctx, cc.ID, "", nil); err != nil {
			return nil, err
		}
	}
	s.emailReveal(ctx, contribs)
	s.sendCobuyThankYou(ctx, ts, m.ItemID, contribs)
	return gen.ConfirmMatch200JSONResponse(toGenMatch(m, contacts)), nil
}

// sendCobuyThankYou fires the owner→giver thank-you note (#22) to each co-buy
// contributor once their match reaches both_confirmed — the reveal point (#113).
// It runs exactly once per completed match (the caller reaches here only on the
// single completing confirm, #36), sending once per participating contributor —
// not per pledge. The note reuses the single-reserve resolution and rendering
// (deliverThankYou): the two-level template, {item}-only token, and no giver
// identity, so anonymity is unchanged. Best-effort throughout: a load or send
// failure is logged and swallowed so the completed match stands regardless.
func (s *Server) sendCobuyThankYou(ctx context.Context, ts storage.TenantStore, itemID string, contribs []storage.Contribution) {
	if len(contribs) == 0 {
		return
	}
	item, err := ts.Items().Get(ctx, itemID)
	if err != nil {
		s.logger.ErrorContext(ctx, "co-buy thank-you: item load failed (ignored)", "item_id", itemID, "error", err)
		return
	}
	list, err := ts.Lists().Get(ctx, item.ListID)
	if err != nil {
		s.logger.ErrorContext(ctx, "co-buy thank-you: list load failed (ignored)", "item_id", item.ID, "error", err)
		return
	}
	for _, c := range contribs {
		s.deliverThankYou(ctx, item, list, c.ContactEmail, "contribution_id", c.ID)
	}
}

// resolveMatchActor authenticates a match action from the token in the capability
// header, accepting EITHER the contribution's capability token (same-browser) or
// its scoped, expiring match-action token (cross-device, #96), and returns the
// contribution only when it belongs to matchID. The cap-token path enforces the
// same participation fence, so a valid capability token from a DIFFERENT match
// cannot read or act on this one. The scoped token additionally must not be expired.
func (s *Server) resolveMatchActor(ctx context.Context, ts storage.TenantStore, raw, matchID string) (storage.Contribution, error) {
	if raw == "" {
		return storage.Contribution{}, errTokenUnauthorized
	}
	hash := token.Hash(raw)

	// Capability token (same-browser).
	c, err := ts.Contributions().ByTokenHash(ctx, hash)
	if err == nil {
		if c.MatchID == nil || *c.MatchID != matchID {
			return storage.Contribution{}, errActorNotInMatch
		}
		return c, nil
	}
	if !errors.Is(err, storage.ErrNotFound) {
		return storage.Contribution{}, err
	}

	// Scoped match-action token (cross-device).
	c, err = ts.Contributions().ByMatchActionTokenHash(ctx, hash)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return storage.Contribution{}, errTokenUnauthorized
		}
		return storage.Contribution{}, err
	}
	if c.MatchID == nil || *c.MatchID != matchID {
		return storage.Contribution{}, errActorNotInMatch
	}
	if c.MatchActionTokenExpiresAt != nil && !s.clock.Now().Before(*c.MatchActionTokenExpiresAt) {
		return storage.Contribution{}, errActionTokenExpired
	}
	return c, nil
}

var (
	errActorNotInMatch    = errors.New("token is not part of that match")
	errActionTokenExpired = errors.New("match-action token expired")
)

// GetMatch returns a match's state so the /cobuy handshake can load it — including
// cross-device via the scoped token. Authorized by resolveMatchActor (cap or scoped
// token, scoped to this match). Contacts are revealed only once both_confirmed,
// through the same toGenMatch boundary as confirmMatch.
func (s *Server) GetMatch(ctx context.Context, req gen.GetMatchRequestObject) (gen.GetMatchResponseObject, error) {
	ts, _, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	raw := capTokenFromContext(ctx)
	if _, err := s.resolveMatchActor(ctx, ts, raw, req.MatchId); err != nil {
		switch {
		case errors.Is(err, errTokenUnauthorized):
			return gen.GetMatch401ApplicationProblemPlusJSONResponse{
				UnauthorizedApplicationProblemPlusJSONResponse: unauthorized("invalid capability token"),
			}, nil
		case errors.Is(err, errActorNotInMatch):
			return gen.GetMatch404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("match not found"),
			}, nil
		case errors.Is(err, errActionTokenExpired):
			return gen.GetMatch410ApplicationProblemPlusJSONResponse(
				problemDetail(410, "this confirmation link has expired"),
			), nil
		}
		return nil, err
	}
	m, err := ts.Matches().Get(ctx, req.MatchId)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.GetMatch404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("match not found"),
			}, nil
		}
		return nil, err
	}
	// Contacts only when both_confirmed; toGenMatch is the boundary, but gather them
	// only in that state so we never even read a contact early.
	var contacts []string
	if m.State == storage.MatchBothConfirmed {
		for _, id := range m.ContributionIDs {
			cc, err := ts.Contributions().Get(ctx, id)
			if err != nil {
				return nil, err
			}
			contacts = append(contacts, cc.ContactEmail)
		}
	}
	return gen.GetMatch200JSONResponse(toGenMatch(m, contacts)), nil
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
		// The released contribution's scoped token is spent; clear it so a re-pledge
		// into a new match starts clean (no stale residue).
		if err := ts.Contributions().SetMatchActionToken(ctx, c.ID, "", nil); err != nil {
			return err
		}
	}
	m.State = storage.MatchDeclined
	if _, err := ts.Matches().Update(ctx, m); err != nil {
		return err
	}
	return nil
}

// emailMatch sends each party their own scoped confirm/decline link (from links,
// keyed by contribution id). The notice reveals nothing about the other parties —
// only that the item is funded and can be confirmed.
func (s *Server) emailMatch(ctx context.Context, contribs []storage.Contribution, links map[string]string) {
	for _, c := range contribs {
		body := "A group gift you pledged toward is now fully funded. " +
			"Confirm or decline the group buy: " + links[c.ID]
		if err := s.email.Send(ctx, email.Message{
			To: c.ContactEmail, Subject: "A co-buying match is proposed", Body: body,
		}); err != nil {
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
