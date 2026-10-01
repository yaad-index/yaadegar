import { redirect } from '@sveltejs/kit';
import { safeReturnTo } from '$lib/server/returnTo';
import { clearSession } from '$lib/server/session';
import type { RequestHandler } from './$types';

// An optional ?return_to= is passed on to the login page, so "sign in again" can
// come back to where it was asked for. It goes through safeReturnTo like every
// other return path, so it can only ever be a local path.
export const POST: RequestHandler = ({ cookies, url }) => {
	clearSession(cookies, url.protocol === 'https:');
	const returnTo = safeReturnTo(url.searchParams.get('return_to'));
	redirect(303, returnTo ? `/login?return_to=${encodeURIComponent(returnTo)}` : '/login');
};
