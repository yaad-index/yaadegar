import type { Cookies } from '@sveltejs/kit';

// yaadegar_caps holds an anonymous giver's one-time reservation capability tokens.
// It is httpOnly so the tokens never reach client JS (ADR-0006 §4): the browser
// that made a reservation can release it later without the token ever being
// scriptable. The map is nested by share slug then item id — the public giver
// surface is reached by an opaque share slug and never exposes the internal list
// id, so the slug is the natural per-list key. Release needs the reservation id
// (that is the release path), so each entry stores it alongside the token.
export const CAPS_COOKIE = 'yaadegar_caps';

export interface CapEntry {
	reservation_id: string;
	token: string;
}

type CapsMap = Record<string, Record<string, CapEntry>>;

// A year: reservations live until released or auto-expired, so the browser should
// keep the release capability for the life of a typical gifting window.
const MAX_AGE_SECONDS = 60 * 60 * 24 * 365;

function read(cookies: Cookies): CapsMap {
	const raw = cookies.get(CAPS_COOKIE);
	if (!raw) return {};
	try {
		const parsed: unknown = JSON.parse(raw);
		return parsed && typeof parsed === 'object' ? (parsed as CapsMap) : {};
	} catch {
		return {};
	}
}

function persist(cookies: Cookies, map: CapsMap, secure: boolean) {
	// Prune empty per-list maps so the cookie shrinks back as reservations are
	// released, and drop the cookie entirely once nothing is held.
	for (const slug of Object.keys(map)) {
		if (Object.keys(map[slug]).length === 0) delete map[slug];
	}
	if (Object.keys(map).length === 0) {
		// Match the attributes used when setting, so the browser reliably removes the
		// cookie (a Secure/path mismatch can leave a stale cookie in place).
		cookies.delete(CAPS_COOKIE, { path: '/', httpOnly: true, sameSite: 'lax', secure });
		return;
	}
	cookies.set(CAPS_COOKIE, JSON.stringify(map), {
		path: '/',
		httpOnly: true,
		sameSite: 'lax',
		secure,
		maxAge: MAX_AGE_SECONDS
	});
}

// capsForList returns this browser's { itemId → CapEntry } for one shared list.
export function capsForList(cookies: Cookies, shareSlug: string): Record<string, CapEntry> {
	return read(cookies)[shareSlug] ?? {};
}

// addCap records the capability for a freshly created reservation.
export function addCap(
	cookies: Cookies,
	shareSlug: string,
	itemId: string,
	entry: CapEntry,
	secure: boolean
) {
	const map = read(cookies);
	(map[shareSlug] ??= {})[itemId] = entry;
	persist(cookies, map, secure);
}

// removeCap forgets a capability once its reservation is released (or gone stale).
export function removeCap(cookies: Cookies, shareSlug: string, itemId: string, secure: boolean) {
	const map = read(cookies);
	if (map[shareSlug]?.[itemId]) {
		delete map[shareSlug][itemId];
		persist(cookies, map, secure);
	}
}
