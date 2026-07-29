import type { Handle } from '@sveltejs/kit';
import { readSession } from '$lib/server/session';

// Every request carries the tenant host (for backend Host-based routing) and the
// owner token from the session cookie (if any). Load/actions build a backend
// client from these; the token stays server-side.
export const handle: Handle = async ({ event, resolve }) => {
	event.locals.host = event.request.headers.get('host') ?? event.url.host;
	event.locals.token = readSession(event.cookies);
	return resolve(event);
};
