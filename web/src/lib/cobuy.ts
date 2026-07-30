import type { components } from '$lib/api/schema';

type Match = components['schemas']['Match'];

export type MatchViewState = 'proposed' | 'both_confirmed' | 'declined' | 'done' | 'unknown';

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

export function matchView(m: Match | null | undefined): MatchView {
	const state = (m?.state ?? 'unknown') as MatchViewState;
	return {
		state,
		participants: m?.contribution_ids?.length ?? 0,
		// Reveal contacts only at both_confirmed — never for proposed/declined/done.
		contacts: state === 'both_confirmed' ? (m?.contacts ?? []) : [],
		// A single decline dissolves the whole match and releases every pledge.
		released: state === 'declined'
	};
}
