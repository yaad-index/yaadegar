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

// backendPostRaw POSTs a raw body with an explicit Content-Type to a non-schema
// backend endpoint — e.g. the list import upload (#26). Same host + bearer forwarding.
export function backendPostRaw(opts: {
	host: string;
	token?: string;
	path: string;
	contentType: string;
	body: string;
}): Promise<Response> {
	const headers: Record<string, string> = {
		'x-forwarded-host': opts.host,
		'content-type': opts.contentType
	};
	if (opts.token) headers.authorization = `Bearer ${opts.token}`;
	return fetch(BACKEND_ORIGIN + opts.path, { method: 'POST', headers, body: opts.body });
}

// backendOAuthPassthrough is a thin, transparent proxy for the browser-facing OAuth
// redirect endpoints (ADR-0008 Cut 2). The backend has no published port — this
// SvelteKit service is the sole proxy — so the browser reaches /start, /callback,
// and /complete through here. It forwards the RAW inbound browser Cookie header
// (never a server-side token: the OAuth /start→/callback state cookie must reach
// the backend), does NOT follow redirects, and passes the backend's status,
// Location, and every Set-Cookie back verbatim — so the redirects and the
// host-scoped `yaadegar_session` cookie the backend sets flow to the browser
// untouched.
export async function backendOAuthPassthrough(opts: {
	path: string;
	cookie: string | null;
	host: string;
}): Promise<Response> {
	const headers: Record<string, string> = { 'x-forwarded-host': opts.host };
	if (opts.cookie) headers.cookie = opts.cookie;
	const res = await fetch(BACKEND_ORIGIN + opts.path, {
		method: 'GET',
		headers,
		redirect: 'manual'
	});
	const out = new Headers();
	const location = res.headers.get('location');
	if (location) out.set('location', location);
	const contentType = res.headers.get('content-type');
	if (contentType) out.set('content-type', contentType);
	for (const c of res.headers.getSetCookie()) out.append('set-cookie', c);
	return new Response(res.body, { status: res.status, headers: out });
}
