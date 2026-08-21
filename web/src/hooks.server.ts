import type { Handle } from '@sveltejs/kit';
import { readSession } from '$lib/server/session';
import { reportVersionSkew } from '$lib/server/version';

// Read the backend build version once at server startup and log loudly on a
// mismatched published pair (ADR-0014 §3). Fire-and-forget: a slow or unreachable
// backend must not block startup, and skew is logged, never fatal — refusing to start
// would turn a staged rollout into an outage.
void reportVersionSkew();

// Every request carries the tenant host (for backend Host-based routing) and the
// owner token from the session cookie (if any). Load/actions build a backend client
// from these; the token stays server-side. Admin is a capability on the owner
// account (ADR-0010), so the /admin surface reuses this same owner session.
export const handle: Handle = async ({ event, resolve }) => {
	event.locals.host = event.request.headers.get('host') ?? event.url.host;
	event.locals.token = readSession(event.cookies);
	return resolve(event);
};
