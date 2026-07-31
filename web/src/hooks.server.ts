import type { Handle } from '@sveltejs/kit';
import { readSession } from '$lib/server/session';
import { readAdminSession } from '$lib/server/adminsession';

// Every request carries the tenant host (for backend Host-based routing) and the
// owner token from the session cookie (if any). The instance-admin token is read
// from its own separate cookie and kept distinct — the /admin surface uses it, the
// owner surface never does. Load/actions build a backend client from these; the
// tokens stay server-side.
export const handle: Handle = async ({ event, resolve }) => {
	event.locals.host = event.request.headers.get('host') ?? event.url.host;
	event.locals.token = readSession(event.cookies);
	event.locals.adminToken = readAdminSession(event.cookies);
	return resolve(event);
};
