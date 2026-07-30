import { describe, it, expect } from 'vitest';
import { matchView } from './cobuy';

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

	it('is defensive about a missing/blank match', () => {
		expect(matchView(null)).toEqual({
			state: 'unknown',
			participants: 0,
			contacts: [],
			released: false
		});
	});
});
