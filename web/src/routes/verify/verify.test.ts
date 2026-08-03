import { describe, it, expect, vi, beforeEach } from 'vitest';

// ADR-0012 cut 1a: the /verify action posts the token, and on success auto-logs-in
// (sets the session cookie, redirects to the app). It surfaces the backend's generic
// reason on failure and guards a missing token.

const post = vi.fn();
vi.mock('$lib/server/api', () => ({ backendClient: () => ({ POST: post }) }));

const setSession = vi.fn();
vi.mock('$lib/server/session', () => ({ setSession: (...args: unknown[]) => setSession(...args) }));

import { actions } from './+page.server';

type Fn = (e: {
	request: Request;
	cookies: Record<string, unknown>;
	locals: { host: string };
	url: URL;
}) => Promise<unknown>;
const verify = (actions as unknown as { default: Fn }).default;

function ev(fields: Record<string, string>) {
	const fd = new FormData();
	for (const [k, v] of Object.entries(fields)) fd.set(k, v);
	return {
		request: { formData: async () => fd } as unknown as Request,
		cookies: {},
		locals: { host: 't.example' },
		url: new URL('https://t.example/verify')
	};
}

describe('verify action (ADR-0012 cut 1a)', () => {
	beforeEach(() => {
		post.mockReset();
		setSession.mockReset();
	});

	it('on success sets the session cookie and redirects to the app (auto-login)', async () => {
		post.mockResolvedValue({ data: { access_token: 'jwt', expires_in: 3600 }, error: undefined });
		await expect(verify(ev({ token: 'tok' }))).rejects.toMatchObject({
			status: 303,
			location: '/'
		});
		expect(setSession).toHaveBeenCalledWith({}, 'jwt', 3600, true);
	});

	it('surfaces the backend reason on an invalid/expired/used token', async () => {
		post.mockResolvedValue({
			data: undefined,
			error: { detail: 'this verification link is invalid or has expired' }
		});
		const res = (await verify(ev({ token: 'bad' }))) as { data: { error: string } };
		expect(setSession).not.toHaveBeenCalled();
		expect(res.data.error).toContain('invalid or has expired');
	});

	it('rejects a missing token without calling the backend', async () => {
		const res = (await verify(ev({}))) as { data: { error: string } };
		expect(post).not.toHaveBeenCalled();
		expect(res.data.error).toContain('missing its token');
	});
});
