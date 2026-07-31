import createClient, { type Client } from 'openapi-fetch';
import { env } from '$env/dynamic/private';
import type { paths } from '$lib/api/schema';

// Where the backend lives (server-side only). Overridable per deployment; defaults
// to the local backend for development.
const BACKEND_ORIGIN = env.BACKEND_ORIGIN ?? 'http://localhost:8080';

// backendClient builds a per-request typed client that targets the backend and
// forwards the tenant host via X-Forwarded-Host (the backend resolves tenant by it —
// the standard reverse-proxy pattern; Node's fetch forbids overriding the outbound
// Host header, and this SvelteKit server is the sole trusted caller), optionally
// attaching the owner's bearer token. Every owner-surface call goes through the
// server so the JWT stays in the httpOnly cookie and never reaches the browser
// (ADR-0006 §4).
export function backendClient(opts: { host: string; token?: string }): Client<paths> {
	const client = createClient<paths>({ baseUrl: BACKEND_ORIGIN });
	client.use({
		onRequest({ request }) {
			request.headers.set('x-forwarded-host', opts.host);
			if (opts.token) request.headers.set('authorization', `Bearer ${opts.token}`);
			return request;
		}
	});
	return client;
}

// backendGetRaw is a raw (untyped) GET against the backend, for file-oriented
// endpoints outside the generated OpenAPI schema — e.g. the list export download
// (#26). Same tenant-host + bearer forwarding as backendClient; the caller streams
// the returned Response back to the browser.
export function backendGetRaw(opts: {
	host: string;
	token?: string;
	path: string;
}): Promise<Response> {
	const headers: Record<string, string> = { 'x-forwarded-host': opts.host };
	if (opts.token) headers.authorization = `Bearer ${opts.token}`;
	return fetch(BACKEND_ORIGIN + opts.path, { headers });
}
