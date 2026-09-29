import { describe, it, expect, vi, afterEach } from 'vitest';

vi.mock('$env/dynamic/private', () => ({ env: { BACKEND_ORIGIN: 'http://backend.test' } }));

import { backendClient } from './api';

afterEach(() => vi.restoreAllMocks());

async function sentHeaders(call: () => Promise<unknown>): Promise<Headers> {
	const fetchMock = vi
		.spyOn(globalThis, 'fetch')
		.mockResolvedValue(new Response('{}', { headers: { 'content-type': 'application/json' } }));
	await call();
	const [input, init] = fetchMock.mock.calls[0];
	return input instanceof Request ? input.headers : new Headers(init?.headers);
}

const locals = { host: 'alice.example.test', token: 'owner-jwt', clientIP: '203.0.113.5' };

describe('backendClient', () => {
	it('sends the tenant host, the owner token and the client address', async () => {
		const h = await sentHeaders(() => backendClient(locals).GET('/api/v1/me'));
		expect(h.get('x-forwarded-host')).toBe('alice.example.test');
		expect(h.get('authorization')).toBe('Bearer owner-jwt');
		expect(h.get('x-forwarded-for')).toBe('203.0.113.5');
	});

	it('anonymous calls carry the client address but never the owner token', async () => {
		const h = await sentHeaders(() => backendClient(locals, { anonymous: true }).GET('/api/v1/me'));
		expect(h.get('authorization')).toBeNull();
		expect(h.get('x-forwarded-for')).toBe('203.0.113.5');
	});

	it('sends no forwarded address when none is known', async () => {
		const h = await sentHeaders(() =>
			backendClient({ host: 'alice.example.test' }).GET('/api/v1/me')
		);
		expect(h.get('x-forwarded-for')).toBeNull();
	});
});
