import type { Handle } from '@sveltejs/kit';
import { readSession } from '$lib/server/session';

// Every request carries the tenant host (for backend Host-based routing) and the
// owner token from the session cookie (if any). Load/actions build a backend client
// from these; the token stays server-side. Admin is a capability on the owner
// account (ADR-0010), so the /admin surface reuses this same owner session.
export const handle: Handle = async ({ event, resolve }) => {
	event.locals.host = event.request.headers.get('host') ?? event.url.host;
	event.locals.token = readSession(event.cookies);
	return resolve(event);
};
