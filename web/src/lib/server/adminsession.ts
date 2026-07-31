import type { Cookies } from '@sveltejs/kit';

// The instance-admin (superadmin) session cookie. It is DELIBERATELY separate from
// the owner SESSION_COOKIE (session.ts): the admin surface authorizes only on the
// backend's requireAdmin superadmin path, and its session must never be confused
// with — or accepted as — an owner session (ADR-0009 Cut 1b). httpOnly, so the JWT
// never reaches client JS.
export const ADMIN_SESSION_COOKIE = 'yaadegar_admin_session';

export function setAdminSession(
	cookies: Cookies,
	token: string,
	maxAgeSeconds: number,
	secure: boolean
) {
	cookies.set(ADMIN_SESSION_COOKIE, token, {
		path: '/',
		httpOnly: true,
		sameSite: 'lax',
		secure,
		maxAge: maxAgeSeconds
	});
}

export function clearAdminSession(cookies: Cookies) {
	cookies.delete(ADMIN_SESSION_COOKIE, { path: '/' });
}

export function readAdminSession(cookies: Cookies): string | undefined {
	return cookies.get(ADMIN_SESSION_COOKIE);
}
