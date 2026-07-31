import { describe, it, expect, vi, afterEach } from 'vitest';
import type { Cookies } from '@sveltejs/kit';

// api.ts imports the SvelteKit $env virtual module, which isn't present under bare
// vitest — stub it so the passthrough helper is unit-testable in isolation.
vi.mock('$env/dynamic/private', () => ({ env: { BACKEND_ORIGIN: 'http://backend.test' } }));

import { backendOAuthPassthrough } from './api';
import { SESSION_COOKIE, readSession } from './session';

// --- A2: the passthrough forwards the browser cookie and preserves redirects +
// Set-Cookie verbatim, without following redirects. ---------------------------

afterEach(() => vi.restoreAllMocks());

describe('backendOAuthPassthrough', () => {
	it('forwards the inbound browser Cookie header (not a server token) to the backend', async () => {
		const fetchMock = vi
			.spyOn(globalThis, 'fetch')
			.mockResolvedValue(new Response(null, { status: 302, headers: { location: '/' } }));
		await backendOAuthPassthrough({
			path: '/api/v1/auth/oauth/google/callback?code=x&state=y',
			cookie: 'yaadegar_oauth_state=abc',
			host: 'alice.example.test'
		});
		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [, init] = fetchMock.mock.calls[0];
		const headers = init?.headers as Record<string, string>;
		expect(headers.cookie).toBe('yaadegar_oauth_state=abc');
		// A server-side Authorization token must NOT be substituted for the state cookie.
		expect(headers.authorization).toBeUndefined();
		expect(init?.redirect).toBe('manual'); // never follow the provider/tenant redirects
	});

	it('passes the backend status and Location back verbatim', async () => {
		vi.spyOn(globalThis, 'fetch').mockResolvedValue(
			new Response(null, {
				status: 302,
				headers: { location: 'https://alice.example.test/api/v1/auth/oauth/complete?ticket=t' }
			})
		);
		const res = await backendOAuthPassthrough({
			path: '/api/v1/auth/oauth/google/callback?code=x&state=y',
			cookie: null,
			host: 'fixed.example.test'
		});
		expect(res.status).toBe(302);
		expect(res.headers.get('location')).toBe(
			'https://alice.example.test/api/v1/auth/oauth/complete?ticket=t'
		);
	});

	it('passes every Set-Cookie back verbatim so state + session cookies round-trip', async () => {
		const backendHeaders = new Headers();
		backendHeaders.append(
			'set-cookie',
			'yaadegar_oauth_state=s; Path=/api/v1/auth/oauth; HttpOnly; Secure; SameSite=Lax'
		);
		backendHeaders.append(
			'set-cookie',
			'yaadegar_session=jwt; Path=/; HttpOnly; Secure; SameSite=Lax'
		);
		backendHeaders.set('location', '/');
		vi.spyOn(globalThis, 'fetch').mockResolvedValue(
			new Response(null, { status: 302, headers: backendHeaders })
		);
		const res = await backendOAuthPassthrough({
			path: '/api/v1/auth/oauth/complete?ticket=t',
			cookie: null,
			host: 'alice.example.test'
		});
		const setCookies = res.headers.getSetCookie();
		expect(setCookies.some((c) => c.startsWith('yaadegar_oauth_state='))).toBe(true);
		expect(setCookies.some((c) => c.startsWith('yaadegar_session='))).toBe(true);
	});
});

// --- A1: the session cookie the backend /complete sets (yaadegar_session, raw
// JWT) is the SAME cookie the SvelteKit session layer reads, so an OAuth-completed
// session authenticates a subsequent request through the frontend. -------------

describe('OAuth session adoption', () => {
	it('SESSION_COOKIE matches the backend-set cookie name', () => {
		// Guards against future drift: the unified-cookie model depends on this name.
		expect(SESSION_COOKIE).toBe('yaadegar_session');
	});

	it('readSession lifts the backend-set yaadegar_session cookie into the token', () => {
		const cookies = {
			get: (name: string) => (name === SESSION_COOKIE ? 'oauth.issued.jwt' : undefined)
		} as unknown as Cookies;
		// This is exactly what hooks.server.ts does to populate locals.token — so a
		// browser arriving from /complete with the backend-set cookie is authenticated.
		expect(readSession(cookies)).toBe('oauth.issued.jwt');
	});
});
