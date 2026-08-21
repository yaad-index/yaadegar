import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { WEB_VERSION, fetchBackendVersion } from '$lib/server/version';

// GET /version — the frontend and backend build versions as a pair (ADR-0014 §3), so
// one unauthenticated poll shows a mismatched image pair directly, rather than an
// operator fetching /api/v1/version and comparing it against the web build by eye.
//
// This is the web service's OWN surface, not the backend passthrough: it lives at
// /version (not under /api/v1/), so it is served here and reports web=this image's
// stamp, api=whatever the backend answers now. Unauthenticated by design — it exists
// to be polled by monitoring that holds no credentials, and it carries only versions.
export const GET: RequestHandler = async () => {
	return json({ web: WEB_VERSION, api: await fetchBackendVersion() });
};
