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

// Clearing must carry the SAME `secure` as the cookie was set with — derived from
// the request protocol, exactly like setSession — not SvelteKit's default. The
// default is Secure for any non-localhost host, so over plain http (e.g. the
// docker-compose LAN demo) the clear cookie would be Secure, the browser would
// refuse it, and the session cookie (set without Secure) would survive: logout
// silently leaves you signed in. Over https both are Secure and it already worked.
export function clearSession(cookies: Cookies, secure: boolean) {
	cookies.delete(SESSION_COOKIE, { path: '/', secure });
}

export function readSession(cookies: Cookies): string | undefined {
	return cookies.get(SESSION_COOKIE);
}
