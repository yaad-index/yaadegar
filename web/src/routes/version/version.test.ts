import { describe, it, expect, vi } from 'vitest';

// The /version route is thin: it pairs the web stamp with whatever the backend reports
// now. Mock the version module (its $env read is exercised in version.test.ts) so this
// asserts the route's shape — both versions, as a pair — without a real env or network.
vi.mock('$lib/server/version', () => ({
	WEB_VERSION: 'web-9.9.9',
	fetchBackendVersion: vi.fn(async () => 'api-1.2.3')
}));

import { GET } from './+server';

describe('GET /version', () => {
	it('returns the web and backend versions as a pair', async () => {
		const res = await (GET as unknown as (e: unknown) => Promise<Response>)({});
		expect(res.status).toBe(200);
		expect(await res.json()).toEqual({ web: 'web-9.9.9', api: 'api-1.2.3' });
	});
});
