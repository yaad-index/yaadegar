import { backendProxy } from '$lib/server/api';
import type { RequestHandler } from './$types';

// Transparent public passthrough for the versioned REST API (#145). External
// clients (mobile, curl, third-party frontends) reach the backend at /api/v1/* on
// the web origin — the backend port is unpublished, so this route is the sole
// public door to the API. The request is forwarded verbatim and the upstream
// response returned unchanged (real status, body, headers), explicitly NOT the
// BFF re-wrapping that swallows the API's reason (cf. #144). `fallback` handles
// every method (GET/POST/PUT/PATCH/DELETE/OPTIONS/HEAD) with one handler. The
// more specific /api/v1/auth/oauth/[...rest] route still wins for OAuth redirects.
export const fallback: RequestHandler = ({ request, url, locals }) =>
	backendProxy({ request, url, host: locals.host });
