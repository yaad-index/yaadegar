import { describe, it, expect } from 'vitest';
import type { Cookies } from '@sveltejs/kit';
import {
	PENDING_COOKIE,
	addPending,
	pendingForList,
	removePending,
	removePendingByReservation
} from './pending';

// A Cookies stand-in that also records the options each set/delete was given — the
// cookie's own lifetime is part of what this module has to get right (#441), and a
// stub that dropped the options would make that untestable while still looking like
// a cookie.
function fakeCookies() {
	const store = new Map<string, string>();
	const sets: { name: string; value: string; opts: Record<string, unknown> }[] = [];
	const deletes: string[] = [];
	const cookies = {
		get: (name: string) => store.get(name),
		set: (name: string, value: string, opts: Record<string, unknown>) => {
			store.set(name, value);
			sets.push({ name, value, opts });
		},
		delete: (name: string) => {
			store.delete(name);
			deletes.push(name);
		},
		getAll: () => [...store].map(([name, value]) => ({ name, value })),
		serialize: () => ''
	} as unknown as Cookies;
	return { cookies, sets, deletes, store };
}

// Every test states its own clock rather than inheriting Date.now(). A test that
// inherits the value whose handling it is checking cannot see a bug in that handling
// — the deadline comparison is the whole point of this module.
const NOW = Date.parse('2026-03-01T12:00:00Z');
const iso = (msFromNow: number) => new Date(NOW + msFromNow).toISOString();

describe('pending confirmation markers', () => {
	it('round-trips a marker for the browser that made the reservation', () => {
		const { cookies } = fakeCookies();
		addPending(
			cookies,
			'slug-a',
			'item-1',
			{ reservation_id: 'res-1', deadline: iso(30 * 60_000) },
			false,
			NOW
		);
		expect(pendingForList(cookies, 'slug-a', NOW)).toEqual({
			'item-1': { reservation_id: 'res-1', deadline: iso(30 * 60_000) }
		});
	});

	it('keeps lists separate', () => {
		const { cookies } = fakeCookies();
		addPending(
			cookies,
			'slug-a',
			'item-1',
			{ reservation_id: 'res-1', deadline: null },
			false,
			NOW
		);
		expect(pendingForList(cookies, 'slug-b', NOW)).toEqual({});
	});

	it('stores no token, so the marker cannot authorize anything', () => {
		const { cookies, store } = fakeCookies();
		addPending(
			cookies,
			'slug-a',
			'item-1',
			{ reservation_id: 'res-1', deadline: null },
			false,
			NOW
		);
		// Asserted on the serialized cookie, not the typed entry: the point is that
		// nothing token-shaped is written to the browser at all, which a type cannot say.
		const raw = store.get(PENDING_COOKIE) ?? '';
		expect(raw).not.toMatch(/token/i);
		expect(JSON.parse(raw)['slug-a']['item-1']).toEqual({
			reservation_id: 'res-1',
			deadline: null
		});
	});

	it('drops a marker once its confirm deadline has passed', () => {
		const { cookies } = fakeCookies();
		addPending(
			cookies,
			'slug-a',
			'item-1',
			{ reservation_id: 'res-1', deadline: iso(60_000) },
			false,
			NOW
		);
		expect(pendingForList(cookies, 'slug-a', NOW)).not.toEqual({});
		expect(pendingForList(cookies, 'slug-a', NOW + 61_000)).toEqual({});
	});

	it('treats the deadline instant itself as elapsed', () => {
		const { cookies } = fakeCookies();
		const at = iso(60_000);
		addPending(cookies, 'slug-a', 'item-1', { reservation_id: 'res-1', deadline: at }, false, NOW);
		expect(pendingForList(cookies, 'slug-a', Date.parse(at))).toEqual({});
	});

	// The zero-window case: no deadline EXISTS, so the hold waits indefinitely and the
	// marker must too. Treating null as "expire it" would erase a live instruction.
	it('never expires a marker that has no deadline', () => {
		const { cookies } = fakeCookies();
		addPending(
			cookies,
			'slug-a',
			'item-1',
			{ reservation_id: 'res-1', deadline: null },
			false,
			NOW
		);
		const farFuture = NOW + 1000 * 60 * 60 * 24 * 3650;
		expect(pendingForList(cookies, 'slug-a', farFuture)).toEqual({
			'item-1': { reservation_id: 'res-1', deadline: null }
		});
	});

	it('discards a marker whose deadline cannot be read', () => {
		const { cookies, store } = fakeCookies();
		store.set(
			PENDING_COOKIE,
			JSON.stringify({ 'slug-a': { 'item-1': { reservation_id: 'r', deadline: 'not-a-date' } } })
		);
		expect(pendingForList(cookies, 'slug-a', NOW)).toEqual({});
	});

	it('survives a corrupt cookie rather than throwing at the giver', () => {
		const { cookies, store } = fakeCookies();
		store.set(PENDING_COOKIE, '{not json');
		expect(pendingForList(cookies, 'slug-a', NOW)).toEqual({});
	});

	it('forgets one marker without disturbing the others', () => {
		const { cookies } = fakeCookies();
		addPending(
			cookies,
			'slug-a',
			'item-1',
			{ reservation_id: 'res-1', deadline: null },
			false,
			NOW
		);
		addPending(
			cookies,
			'slug-a',
			'item-2',
			{ reservation_id: 'res-2', deadline: null },
			false,
			NOW
		);
		removePending(cookies, 'slug-a', 'item-1', false, NOW);
		expect(Object.keys(pendingForList(cookies, 'slug-a', NOW))).toEqual(['item-2']);
	});

	it('clears the marker for a reservation confirmed in this browser, across lists', () => {
		const { cookies } = fakeCookies();
		addPending(
			cookies,
			'slug-a',
			'item-1',
			{ reservation_id: 'res-1', deadline: null },
			false,
			NOW
		);
		addPending(
			cookies,
			'slug-b',
			'item-9',
			{ reservation_id: 'res-9', deadline: null },
			false,
			NOW
		);
		removePendingByReservation(cookies, 'res-9', false, NOW);
		expect(pendingForList(cookies, 'slug-b', NOW)).toEqual({});
		expect(Object.keys(pendingForList(cookies, 'slug-a', NOW))).toEqual(['item-1']);
	});

	it('is a no-op for a reservation this browser never marked', () => {
		const { cookies, sets } = fakeCookies();
		addPending(
			cookies,
			'slug-a',
			'item-1',
			{ reservation_id: 'res-1', deadline: null },
			false,
			NOW
		);
		const before = sets.length;
		removePendingByReservation(cookies, 'res-unknown', false, NOW);
		expect(sets.length).toBe(before);
		expect(Object.keys(pendingForList(cookies, 'slug-a', NOW))).toEqual(['item-1']);
	});

	it('drops the cookie entirely once nothing is pending', () => {
		const { cookies, deletes } = fakeCookies();
		addPending(
			cookies,
			'slug-a',
			'item-1',
			{ reservation_id: 'res-1', deadline: null },
			false,
			NOW
		);
		removePending(cookies, 'slug-a', 'item-1', false, NOW);
		expect(deletes).toContain(PENDING_COOKIE);
		expect(pendingForList(cookies, 'slug-a', NOW)).toEqual({});
	});

	describe('cookie lifetime', () => {
		it('outlives its longest-lived marker and no longer', () => {
			const { cookies, sets } = fakeCookies();
			addPending(
				cookies,
				'slug-a',
				'item-1',
				{ reservation_id: 'res-1', deadline: iso(10 * 60_000) },
				false,
				NOW
			);
			addPending(
				cookies,
				'slug-a',
				'item-2',
				{ reservation_id: 'res-2', deadline: iso(45 * 60_000) },
				false,
				NOW
			);
			expect(sets.at(-1)?.opts.maxAge).toBe(45 * 60);
		});

		it('pins to the long backstop when a marker has no deadline', () => {
			const { cookies, sets } = fakeCookies();
			addPending(
				cookies,
				'slug-a',
				'item-1',
				{ reservation_id: 'res-1', deadline: iso(10 * 60_000) },
				false,
				NOW
			);
			addPending(
				cookies,
				'slug-a',
				'item-2',
				{ reservation_id: 'res-2', deadline: null },
				false,
				NOW
			);
			expect(sets.at(-1)?.opts.maxAge).toBe(60 * 60 * 24 * 365);
		});

		it('is written httpOnly, so it never reaches client JS', () => {
			const { cookies, sets } = fakeCookies();
			addPending(
				cookies,
				'slug-a',
				'item-1',
				{ reservation_id: 'res-1', deadline: null },
				false,
				NOW
			);
			expect(sets.at(-1)?.opts).toMatchObject({ httpOnly: true, sameSite: 'lax', path: '/' });
		});
	});
});
