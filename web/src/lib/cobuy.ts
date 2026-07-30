import type { components } from '$lib/api/schema';

type Match = components['schemas']['Match'];
type PublicItem = components['schemas']['PublicItem'];

// chipInAllowed reports whether a giver may co-buy an item (#100): it must be
// priced (a positive amount to split) AND the owner must still allow co-buying
// (allow_cobuy resolved true). Availability (available vs co_buying) is handled
// separately in the view; this is only the priced-AND-allowed gate.
export function chipInAllowed(item: Pick<PublicItem, 'price' | 'allow_cobuy'>): boolean {
	return !!item.price && (item.price.amount_minor ?? 0) > 0 && item.allow_cobuy === true;
}

export type MatchViewState =
	'proposed' | 'both_confirmed' | 'declined' | 'done' | 'expired' | 'unknown';

// MatchView is the giver-facing shape of a co-buying match after a confirm/decline.
// It is the anonymity choke point on the client side: contacts are surfaced ONLY
// when the match is genuinely both_confirmed, regardless of what the payload
// carries. (The backend already withholds contacts pre-both_confirmed in
// toGenMatch; this is defense in depth so a stray payload can never leak them.)
export interface MatchView {
	state: MatchViewState;
	participants: number;
	contacts: string[];
	released: boolean;
}

// matchLoadFailureState classifies a failed GET-match on the handshake load. A
// scoped (emailed) token is CLEARED once the match resolves, so a 401/404 on that
// path most likely means "already resolved — check your email", NOT an error; the
// same-browser cap token keeps working, so its 401/404 is a genuinely bad link.
export function matchLoadFailureState(
	status: number | undefined,
	scoped: boolean
): 'expired' | 'resolved' | 'invalid' | 'error' {
	if (status === 410) return 'expired';
	if (status === 401 || status === 404) return scoped ? 'resolved' : 'invalid';
	return 'error';
}

export function matchView(m: Match | null | undefined): MatchView {
	const state = (m?.state ?? 'unknown') as MatchViewState;
	return {
		state,
		participants: m?.contribution_ids?.length ?? 0,
		// Reveal contacts only at both_confirmed — never for proposed/declined/done.
		contacts: state === 'both_confirmed' ? (m?.contacts ?? []) : [],
		// A decline dissolves the match and releases every pledge; the auto-expiry
		// sweep (#101) drives the same outcome when the confirm window elapses.
		released: state === 'declined' || state === 'expired'
	};
}
