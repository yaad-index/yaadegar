import { describe, it, expect, vi, afterEach } from 'vitest';

// api.ts imports the SvelteKit $env virtual module, which isn't present under bare
// vitest — stub it so the proxy helper is unit-testable in isolation.
vi.mock('$env/dynamic/private', () => ({ env: { BACKEND_ORIGIN: 'http://backend.test' } }));

import { backendProxy } from './api';

afterEach(() => vi.restoreAllMocks());

// Build the (request, url, host) shape the route hands backendProxy.
function inbound(
	path: string,
	init: RequestInit & { host?: string } = {}
): { request: Request; url: URL; host: string } {
	const { host = 'alice.example.test', ...reqInit } = init;
	const url = new URL(`http://alice.example.test${path}`);
	return { request: new Request(url, reqInit), url, host };
}

describe('backendProxy (#145 transparent API passthrough)', () => {
	it('forwards a 2xx status + body verbatim and stamps the tenant host', async () => {
		const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
			new Response('{"id":"list-1"}', {
				status: 200,
				headers: { 'content-type': 'application/json' }
			})
		);

		const res = await backendProxy(
			inbound('/api/v1/lists/list-1', {
				headers: { authorization: 'Bearer client-token' },
				host: 'alice.example.test'
			})
		);

		// Upstream URL keeps the versioned prefix + path verbatim against BACKEND_ORIGIN.
		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [target, init] = fetchMock.mock.calls[0];
		expect(target).toBe('http://backend.test/api/v1/lists/list-1');
		const headers = init?.headers as Headers;
		// Client's Authorization is forwarded (token auth for non-browser clients)...
		expect(headers.get('authorization')).toBe('Bearer client-token');
		// ...and the tenant host is stamped from the real inbound Host.
		expect(headers.get('x-forwarded-host')).toBe('alice.example.test');
		expect(init?.redirect).toBe('manual');

		// Response comes back verbatim.
		expect(res.status).toBe(200);
		expect(res.headers.get('content-type')).toBe('application/json');
		expect(await res.text()).toBe('{"id":"list-1"}');
	});

	it('forwards a 4xx problem+json body + status verbatim (the #144 anti-goal)', async () => {
		const problem = JSON.stringify({
			type: 'about:blank',
			title: 'Bad Request',
			status: 400,
			detail: 'reservation already exists for this item'
		});
		vi.spyOn(globalThis, 'fetch').mockResolvedValue(
			new Response(problem, {
				status: 400,
				headers: { 'content-type': 'application/problem+json' }
			})
		);

		const res = await backendProxy(
			inbound('/api/v1/public/reservations', { method: 'POST', body: '{}' })
		);

		expect(res.status).toBe(400);
		expect(res.headers.get('content-type')).toBe('application/problem+json');
		const parsed = JSON.parse(await res.text());
		// The backend's reason survives — not swallowed into a generic envelope.
		expect(parsed.detail).toBe('reservation already exists for this item');
	});

	it('drops a client-spoofed x-forwarded-host and uses the real inbound host (tenant isolation)', async () => {
		const fetchMock = vi
			.spyOn(globalThis, 'fetch')
			.mockResolvedValue(new Response('{}', { status: 200 }));

		await backendProxy(
			inbound('/api/v1/lists', {
				headers: { 'x-forwarded-host': 'victim.example.test' },
				host: 'alice.example.test'
			})
		);

		const [, init] = fetchMock.mock.calls[0];
		const headers = init?.headers as Headers;
		// The spoofed value must not reach the backend; the trusted host wins.
		expect(headers.get('x-forwarded-host')).toBe('alice.example.test');
	});

	it('refuses (404) a path outside /api/v1 without calling upstream', async () => {
		const fetchMock = vi.spyOn(globalThis, 'fetch');
		// A crafted url that doesn't sit under the versioned API prefix.
		const url = new URL('http://alice.example.test/internal/health');
		const res = await backendProxy({
			request: new Request(url),
			url,
			host: 'alice.example.test'
		});
		expect(res.status).toBe(404);
		expect(fetchMock).not.toHaveBeenCalled();
	});

	it('sends no body on GET and forwards the method + body on POST', async () => {
		const fetchMock = vi
			.spyOn(globalThis, 'fetch')
			.mockResolvedValue(new Response('{}', { status: 200 }));

		await backendProxy(inbound('/api/v1/lists'));
		expect(fetchMock.mock.calls[0][1]?.method).toBe('GET');
		expect(fetchMock.mock.calls[0][1]?.body).toBeUndefined();

		await backendProxy(
			inbound('/api/v1/items', {
				method: 'POST',
				body: '{"name":"gift"}',
				headers: { 'content-type': 'application/json' }
			})
		);
		const postInit = fetchMock.mock.calls[1][1];
		expect(postInit?.method).toBe('POST');
		expect(new TextDecoder().decode(postInit?.body as ArrayBuffer)).toBe('{"name":"gift"}');
	});

	it('passes every upstream Set-Cookie back verbatim', async () => {
		const upstreamHeaders = new Headers();
		upstreamHeaders.append('set-cookie', 'yaadegar_session=jwt; Path=/; HttpOnly');
		upstreamHeaders.append('set-cookie', 'other=1; Path=/');
		vi.spyOn(globalThis, 'fetch').mockResolvedValue(
			new Response('{}', { status: 200, headers: upstreamHeaders })
		);

		const res = await backendProxy(inbound('/api/v1/auth/login', { method: 'POST', body: '{}' }));
		const cookies = res.headers.getSetCookie();
		expect(cookies.some((c) => c.startsWith('yaadegar_session='))).toBe(true);
		expect(cookies.some((c) => c.startsWith('other='))).toBe(true);
	});

	it('does not forward hop-by-hop request headers', async () => {
		const fetchMock = vi
			.spyOn(globalThis, 'fetch')
			.mockResolvedValue(new Response('{}', { status: 200 }));

		await backendProxy(
			inbound('/api/v1/lists', {
				headers: { connection: 'keep-alive', 'x-real-thing': 'kept' }
			})
		);

		const headers = fetchMock.mock.calls[0][1]?.headers as Headers;
		expect(headers.get('connection')).toBeNull();
		expect(headers.get('x-real-thing')).toBe('kept');
	});
});
