import { describe, it, expect, vi } from 'vitest';

// #397 (ADR-0016): Settings loads the account's tokens, and reads the number of
// tokens a password reset revoked from the URL it redirected to. That number is
// only ever shown, and only a whole positive one is.

const get = vi.fn(async (path: string) => {
	if (path === '/api/v1/me/tokens') return { data: [{ id: 't1' }] };
	return { data: undefined };
});
vi.mock('$lib/server/api', () => ({ backendClient: () => ({ GET: get }) }));
vi.mock('$lib/server/session', () => ({ setSession: vi.fn() }));

import { load } from './+page.server';

type LoadFn = (e: { locals: object; url: URL }) => Promise<Record<string, unknown>>;
const run = (query: string) =>
	(load as unknown as LoadFn)({ locals: {}, url: new URL(`https://t.example/settings${query}`) });

describe('settings load (ADR-0016)', () => {
	it('loads the tokens and the expiry choices', async () => {
		const out = await run('');
		expect(get).toHaveBeenCalledWith('/api/v1/me/tokens');
		expect(out.tokens).toEqual([{ id: 't1' }]);
		expect(out.tokenExpiryDays).toEqual([30, 90, 365]);
		expect(out.tokensRevoked).toBe(0);
	});

	it.each([
		['?tokens_revoked=3', 3],
		['?tokens_revoked=0', 0],
		['?tokens_revoked=-2', 0],
		['?tokens_revoked=1.5', 0],
		['?tokens_revoked=abc', 0],
		['?tokens_revoked=%3Cb%3E', 0]
	])('reads %s as %d', async (query, want) => {
		expect((await run(query)).tokensRevoked).toBe(want);
	});
});
