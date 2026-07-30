import { describe, it, expect } from 'vitest';
import { matchView, matchLoadFailureState, chipInAllowed } from './cobuy';

describe('matchLoadFailureState', () => {
	// A scoped (emailed) token is cleared once the match resolves, so a 401/404 on
	// the cross-device path means "already resolved — check your email", never an error.
	it('treats a cleared scoped token (401/404) as resolved, not an error', () => {
		expect(matchLoadFailureState(401, true)).toBe('resolved');
		expect(matchLoadFailureState(404, true)).toBe('resolved');
	});
	it('treats a same-browser cap 401/404 as a genuinely invalid link', () => {
		expect(matchLoadFailureState(401, false)).toBe('invalid');
		expect(matchLoadFailureState(404, false)).toBe('invalid');
	});
	it('maps 410 to expired regardless of token kind', () => {
		expect(matchLoadFailureState(410, true)).toBe('expired');
		expect(matchLoadFailureState(410, false)).toBe('expired');
	});
	it('falls back to error for anything else', () => {
		expect(matchLoadFailureState(502, true)).toBe('error');
		expect(matchLoadFailureState(undefined, false)).toBe('error');
	});
});

describe('chipInAllowed', () => {
	const priced = { amount_minor: 10000, currency: 'EUR' };

	it('allows chip-in for a priced item the owner still allows', () => {
		expect(chipInAllowed({ price: priced, allow_cobuy: true })).toBe(true);
	});
	it('hides chip-in when the owner opted the item out (#100)', () => {
		expect(chipInAllowed({ price: priced, allow_cobuy: false })).toBe(false);
	});
	it('hides chip-in on an unpriced item even when co-buy is allowed', () => {
		expect(chipInAllowed({ price: undefined, allow_cobuy: true })).toBe(false);
		expect(chipInAllowed({ price: { amount_minor: 0, currency: 'EUR' }, allow_cobuy: true })).toBe(
			false
		);
	});
});

describe('matchView', () => {
	// The load-bearing anonymity guard: contacts surface ONLY at both_confirmed.
	it('hides contacts while the match is only proposed', () => {
		const v = matchView({
			state: 'proposed',
			contribution_ids: ['a', 'b', 'c'],
			// Even if a payload wrongly carried contacts, a proposed match must not leak them.
			contacts: ['leak@example.com']
		});
		expect(v.contacts).toEqual([]);
		expect(v.participants).toBe(3);
		expect(v.released).toBe(false);
	});

	it('reveals all participants’ contacts only once both_confirmed', () => {
		const v = matchView({
			state: 'both_confirmed',
			contribution_ids: ['a', 'b'],
			contacts: ['x@example.com', 'y@example.com']
		});
		expect(v.contacts).toEqual(['x@example.com', 'y@example.com']);
		expect(v.participants).toBe(2);
		expect(v.released).toBe(false);
	});

	it('marks a declined match released and reveals nothing', () => {
		const v = matchView({ state: 'declined', contribution_ids: ['a', 'b'], contacts: [] });
		expect(v.released).toBe(true);
		expect(v.contacts).toEqual([]);
	});

	// An auto-expired match (#101) is released just like a decline — the pledges are
	// terminal, so the giver can chip in again — and never reveals contacts.
	it('marks an expired match released and reveals nothing', () => {
		const v = matchView({ state: 'expired', contribution_ids: ['a', 'b'], contacts: [] });
		expect(v.state).toBe('expired');
		expect(v.released).toBe(true);
		expect(v.contacts).toEqual([]);
	});

	it('is defensive about a missing/blank match', () => {
		expect(matchView(null)).toEqual({
			state: 'unknown',
			participants: 0,
			contacts: [],
			released: false
		});
	});
});
