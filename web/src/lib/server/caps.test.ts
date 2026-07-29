import { describe, it, expect } from 'vitest';
import type { Cookies } from '@sveltejs/kit';
import {
	CAPS_COOKIE,
	addCap,
	capsForList,
	addConfirmedCap,
	confirmedCap,
	removeConfirmedCap
} from './caps';

// A minimal in-memory Cookies stand-in — enough for the get/set/delete the caps
// helpers use.
function fakeCookies(): Cookies {
	const store = new Map<string, string>();
	return {
		get: (name: string) => store.get(name),
		set: (name: string, value: string) => store.set(name, value),
		delete: (name: string) => store.delete(name),
		getAll: () => [...store].map(([name, value]) => ({ name, value })),
		serialize: () => ''
	} as unknown as Cookies;
}

describe('confirmed reservation caps', () => {
	it('round-trips a confirmed capability by reservation id', () => {
		const cookies = fakeCookies();
		addConfirmedCap(cookies, { reservation_id: 'res-1', token: 'tok-1' }, false);
		expect(confirmedCap(cookies, 'res-1')).toEqual({ reservation_id: 'res-1', token: 'tok-1' });
	});

	it('forgets a confirmed capability once released', () => {
		const cookies = fakeCookies();
		addConfirmedCap(cookies, { reservation_id: 'res-1', token: 'tok-1' }, false);
		removeConfirmedCap(cookies, 'res-1', false);
		expect(confirmedCap(cookies, 'res-1')).toBeUndefined();
	});

	it('returns undefined for an unknown reservation', () => {
		expect(confirmedCap(fakeCookies(), 'nope')).toBeUndefined();
	});

	// The confirmed namespace must not collide with per-list caps sharing the cookie:
	// a confirmed capability keyed by reservation id must not surface as a list's item
	// cap, and vice-versa.
	it('does not collide with per-list caps in the same cookie', () => {
		const cookies = fakeCookies();
		addCap(cookies, 'my-slug', 'item-1', { reservation_id: 'res-list', token: 'tok-list' }, false);
		addConfirmedCap(cookies, { reservation_id: 'res-conf', token: 'tok-conf' }, false);

		// Each store sees only its own entries.
		expect(capsForList(cookies, 'my-slug')).toEqual({
			'item-1': { reservation_id: 'res-list', token: 'tok-list' }
		});
		expect(confirmedCap(cookies, 'res-conf')).toEqual({
			reservation_id: 'res-conf',
			token: 'tok-conf'
		});
		// The confirmed entry is not visible as a list, and the list entry is not a
		// confirmed cap.
		expect(confirmedCap(cookies, 'item-1')).toBeUndefined();
		expect(capsForList(cookies, 'my-slug')['res-conf']).toBeUndefined();

		// Both live in the one caps cookie.
		expect(cookies.get(CAPS_COOKIE)).toBeTruthy();
	});
});
