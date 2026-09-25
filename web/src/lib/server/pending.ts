import type { Cookies } from '@sveltejs/kit';

// yaadegar_pending remembers, for the browser that made them, which items are held
// as pending_confirmation — so a giver who reloads the list (or comes back to it)
// still sees that they have not finished (#441). The reserve result carries that
// state only for the render that follows the POST, so before this the page forgot
// it the moment it was left.
//
// 🔑 THIS IS NOT A CAPABILITY, and it lives outside caps.ts for that reason rather
// than as a fourth map inside it. An email_confirmed reserve deliberately issues no
// token (ADR-0007 §3), and this marker carries none: there is nothing in it to
// authorize with, so it cannot become a way to release or modify an unconfirmed
// reservation. Filing it next to the capability cookies would make the two look
// interchangeable to the next reader, which is the mistake worth making structurally
// impossible. It is still httpOnly — client JS has no use for it, the server hands
// the page what it needs — so it matches the discipline of ADR-0006 §4 without
// claiming the authority.
//
// It is keyed slug → itemId like capsForList, and additionally stores the
// reservation id so a confirm in THIS browser can clear the marker it belongs to;
// the emailed confirm link carries no share slug or item id, only the reservation
// (see removePendingByReservation).
export const PENDING_COOKIE = 'yaadegar_pending';

export interface PendingEntry {
	reservation_id: string;
	// The instant the confirm window closes, ISO-8601, or null when NO deadline
	// exists. Null is not "unknown": a zero effective confirm window disables the
	// expiry sweep entirely, so the hold waits indefinitely and this marker must
	// too. Anything that treats null as "expire it soon" would drop the instruction
	// out from under a giver whose reservation is still perfectly alive.
	deadline: string | null;
	// The same instant as `deadline`, already rendered for a person by the server in
	// the instance's timezone with that zone named (#438). Display only: `deadline`
	// above stays the machine-readable ISO value and remains the one this module
	// does arithmetic on — expiry and the cookie's max-age both Date.parse it, and a
	// human string would silently make every marker look unparseable, i.e. expired.
	//
	// Absent on markers written before #438 shipped. Such a marker renders its
	// instruction without the "until <time>" clause rather than falling back to a
	// second client-side formatter, which is the divergence #438 exists to remove.
	// The gap is bounded by the confirm window, since the marker dies at its own
	// deadline.
	deadline_display?: string | null;
}

type PendingMap = Record<string, Record<string, PendingEntry>>;

// The backstop lifetime for markers that have no deadline of their own. It matches
// the capability cookie's year, for the same reason: the hold really can outlive any
// shorter guess, so a shorter one would erase a live instruction.
const MAX_AGE_SECONDS = 60 * 60 * 24 * 365;

function read(cookies: Cookies): PendingMap {
	const raw = cookies.get(PENDING_COOKIE);
	if (!raw) return {};
	try {
		const parsed: unknown = JSON.parse(raw);
		return parsed && typeof parsed === 'object' ? (parsed as PendingMap) : {};
	} catch {
		return {};
	}
}

// expired is the authority on whether a marker still stands — NOT the cookie's own
// expiry attribute. The cookie attribute is a backstop the browser applies on its
// own clock and only between requests; this runs on every read, on the server's
// clock, which is the one the expiry sweep uses.
function expired(entry: PendingEntry, now: number): boolean {
	if (entry.deadline === null) return false;
	const at = Date.parse(entry.deadline);
	// An unparseable deadline is treated as expired: the alternative is telling a
	// giver to act by a time nobody can read, and a marker is cheap to lose.
	return Number.isNaN(at) || at <= now;
}

function prune(map: PendingMap, now: number): PendingMap {
	for (const slug of Object.keys(map)) {
		const items = map[slug];
		if (!items || typeof items !== 'object') {
			delete map[slug];
			continue;
		}
		for (const itemId of Object.keys(items)) {
			if (expired(items[itemId], now)) delete items[itemId];
		}
		if (Object.keys(items).length === 0) delete map[slug];
	}
	return map;
}

function persist(cookies: Cookies, map: PendingMap, secure: boolean, now: number) {
	prune(map, now);
	if (Object.keys(map).length === 0) {
		// Same attributes as the set below, so the browser reliably drops it — a
		// path/Secure mismatch can leave a stale cookie in place.
		cookies.delete(PENDING_COOKIE, { path: '/', httpOnly: true, sameSite: 'lax', secure });
		return;
	}
	// The cookie outlives its longest-lived marker and no longer. A marker with no
	// deadline pins it to the year backstop, since that hold has no end either.
	let maxAge = 0;
	for (const items of Object.values(map)) {
		for (const entry of Object.values(items)) {
			if (entry.deadline === null) {
				maxAge = MAX_AGE_SECONDS;
				break;
			}
			const secs = Math.ceil((Date.parse(entry.deadline) - now) / 1000);
			if (secs > maxAge) maxAge = secs;
		}
		if (maxAge === MAX_AGE_SECONDS) break;
	}
	cookies.set(PENDING_COOKIE, JSON.stringify(map), {
		path: '/',
		httpOnly: true,
		sameSite: 'lax',
		secure,
		maxAge: Math.min(maxAge, MAX_AGE_SECONDS)
	});
}

// pendingForList returns this browser's still-live { itemId → PendingEntry } for one
// list. Markers past their deadline are never returned, whatever the cookie says.
export function pendingForList(
	cookies: Cookies,
	shareSlug: string,
	now: number = Date.now()
): Record<string, PendingEntry> {
	return prune(read(cookies), now)[shareSlug] ?? {};
}

// addPending records that this browser is the one waiting to confirm `itemId`.
export function addPending(
	cookies: Cookies,
	shareSlug: string,
	itemId: string,
	entry: PendingEntry,
	secure: boolean,
	now: number = Date.now()
) {
	const map = read(cookies);
	(map[shareSlug] ??= {})[itemId] = entry;
	persist(cookies, map, secure, now);
}

// removePending forgets one marker — the hold was confirmed, released, or lapsed.
export function removePending(
	cookies: Cookies,
	shareSlug: string,
	itemId: string,
	secure: boolean,
	now: number = Date.now()
) {
	const map = read(cookies);
	if (map[shareSlug]?.[itemId]) {
		delete map[shareSlug][itemId];
		persist(cookies, map, secure, now);
	}
}

// removePendingByReservation clears the marker for a reservation confirmed in THIS
// browser. The emailed confirm link carries only a one-time token, and the confirm
// response only a reservation id — no share slug, no item — so the marker has to be
// found by scanning, the same way findContribCap resolves a pledge by its id.
//
// Confirming in a DIFFERENT browser than the one that reserved cannot clear anything
// here, and does not: that marker stands until its deadline. Its cost is bounded and
// visible — the original browser keeps saying "check your email" for a hold that is
// already confirmed — and it is the price of keeping this state client-side and
// authority-free. Closing it needs a server-side answer to "does this browser have a
// pending reservation", which needs an identifier for a browser that was deliberately
// given no token (#441 weighs both).
export function removePendingByReservation(
	cookies: Cookies,
	reservationId: string,
	secure: boolean,
	now: number = Date.now()
) {
	const map = read(cookies);
	let hit = false;
	for (const items of Object.values(map)) {
		for (const [itemId, entry] of Object.entries(items)) {
			if (entry.reservation_id === reservationId) {
				delete items[itemId];
				hit = true;
			}
		}
	}
	if (hit) persist(cookies, map, secure, now);
}
