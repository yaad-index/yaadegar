import { describe, it, expect } from 'vitest';
import type { Cookies } from '@sveltejs/kit';
import {
	CAPS_COOKIE,
	addCap,
	capsForList,
	addConfirmedCap,
	confirmedCap,
	removeConfirmedCap,
	COBUY_COOKIE,
	addContribCap,
	contribCapsForList,
	removeContribCap
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

// recordingCookies additionally captures the options passed to set/delete, so the
// cookie attributes and prune-to-delete behaviour can be asserted.
function recordingCookies(): {
	cookies: Cookies;
	sets: Record<string, Record<string, unknown>>;
	deletes: string[];
} {
	const store = new Map<string, string>();
	const sets: Record<string, Record<string, unknown>> = {};
	const deletes: string[] = [];
	const cookies = {
		get: (name: string) => store.get(name),
		set: (name: string, value: string, opts: Record<string, unknown>) => {
			store.set(name, value);
			sets[name] = opts;
		},
		delete: (name: string) => {
			store.delete(name);
			deletes.push(name);
		},
		getAll: () => [...store].map(([name, value]) => ({ name, value })),
		serialize: () => ''
	} as unknown as Cookies;
	return { cookies, sets, deletes };
}

describe('co-buy contribution caps', () => {
	it('persists under the separate COBUY_COOKIE, not the reservation cookie', () => {
		const cookies = fakeCookies();
		addContribCap(cookies, 'slug', 'item-1', { contribution_id: 'c1', token: 't1' }, false);
		expect(cookies.get(COBUY_COOKIE)).toBeTruthy();
		expect(cookies.get(CAPS_COOKIE)).toBeUndefined();
		expect(contribCapsForList(cookies, 'slug')).toEqual({
			'item-1': { contribution_id: 'c1', token: 't1' }
		});
	});

	// The whole reason for a separate cookie: an item can hold BOTH a reservation cap
	// and a contribution cap for one browser (reserve and co-buy are independent
	// backend tracks), and neither must clobber the other.
	it('coexists with a reservation cap for the same slug+itemId', () => {
		const cookies = fakeCookies();
		addCap(cookies, 'slug', 'item-1', { reservation_id: 'r1', token: 'rt' }, false);
		addContribCap(cookies, 'slug', 'item-1', { contribution_id: 'c1', token: 'ct' }, false);

		expect(capsForList(cookies, 'slug')['item-1']).toEqual({ reservation_id: 'r1', token: 'rt' });
		expect(contribCapsForList(cookies, 'slug')['item-1']).toEqual({
			contribution_id: 'c1',
			token: 'ct'
		});
	});

	it('prunes the empty per-list map and drops the cookie when nothing is left', () => {
		const { cookies, deletes } = recordingCookies();
		addContribCap(cookies, 'slug', 'item-1', { contribution_id: 'c1', token: 't1' }, false);
		removeContribCap(cookies, 'slug', 'item-1', false);
		expect(contribCapsForList(cookies, 'slug')).toEqual({});
		expect(cookies.get(COBUY_COOKIE)).toBeUndefined();
		expect(deletes).toContain(COBUY_COOKIE);
	});

	it('sets the cookie httpOnly + sameSite lax + secure (when secure=true)', () => {
		const { cookies, sets } = recordingCookies();
		addContribCap(cookies, 'slug', 'item-1', { contribution_id: 'c1', token: 't1' }, true);
		expect(sets[COBUY_COOKIE]).toMatchObject({ httpOnly: true, sameSite: 'lax', secure: true });
	});
});
