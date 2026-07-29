import type { Cookies } from '@sveltejs/kit';

// The owner session cookie holds the JWT. It is httpOnly (never exposed to client
// JS), lax-samesite, and secure over https (ADR-0006 §4).
export const SESSION_COOKIE = 'yaadegar_session';

export function setSession(
	cookies: Cookies,
	token: string,
	maxAgeSeconds: number,
	secure: boolean
) {
	cookies.set(SESSION_COOKIE, token, {
		path: '/',
		httpOnly: true,
		sameSite: 'lax',
		secure,
		maxAge: maxAgeSeconds
	});
}

export function clearSession(cookies: Cookies) {
	cookies.delete(SESSION_COOKIE, { path: '/' });
}

export function readSession(cookies: Cookies): string | undefined {
	return cookies.get(SESSION_COOKIE);
}
