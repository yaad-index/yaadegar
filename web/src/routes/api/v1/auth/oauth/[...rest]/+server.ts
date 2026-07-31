import { backendOAuthPassthrough } from '$lib/server/api';
import type { RequestHandler } from './$types';

// Transparent proxy for the browser-facing OAuth endpoints (/start, /callback,
// /complete) — the backend isn't externally reachable, so it's fronted here
// (ADR-0008 Cut 2). The full path is forwarded verbatim; the raw browser Cookie
// header carries the state/session cookies; redirects and Set-Cookie pass through
// untouched. See backendOAuthPassthrough for the load-bearing details.
export const GET: RequestHandler = ({ request, url, locals }) =>
	backendOAuthPassthrough({
		path: url.pathname + url.search,
		cookie: request.headers.get('cookie'),
		host: locals.host
	});
