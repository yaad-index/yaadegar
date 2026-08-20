import type { Cookies } from '@sveltejs/kit';
import { defineCookie } from './cookies';

// The owner session cookie holds the JWT. It is httpOnly (never exposed to client
// JS), lax-samesite, and secure over https (ADR-0006 §4). Its set and clear go
// through one owner (defineCookie) so their attributes — including `secure`, the
// axis #238 got wrong — cannot drift apart. `secure` is derived from the request
// protocol by every caller and passed to both set and clear.
export const SESSION_COOKIE = 'yaadegar_session';

const sessionCookie = defineCookie(SESSION_COOKIE, {
	path: '/',
	httpOnly: true,
	sameSite: 'lax'
});

export function setSession(
	cookies: Cookies,
	token: string,
	maxAgeSeconds: number,
	secure: boolean
) {
	sessionCookie.set(cookies, token, { maxAge: maxAgeSeconds, secure });
}

export function clearSession(cookies: Cookies, secure: boolean) {
	sessionCookie.clear(cookies, { secure });
}

export function readSession(cookies: Cookies): string | undefined {
	return sessionCookie.read(cookies);
}
