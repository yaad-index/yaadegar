import { describe, it, expect, vi, beforeEach } from 'vitest';

// ADR-0012 cut 1a: the /verify action posts the token, and on success auto-logs-in
// (sets the session cookie, redirects to the app). It surfaces the backend's generic
// reason on failure and guards a missing token.

const post = vi.fn();
vi.mock('$lib/server/api', () => ({ backendClient: () => ({ POST: post }) }));

const setSession = vi.fn();
vi.mock('$lib/server/session', () => ({ setSession: (...args: unknown[]) => setSession(...args) }));
vi.mock('$lib/server/returnTo', () => ({
	safeReturnTo: (raw?: string | null) =>
		raw && raw.startsWith('/') && !raw.startsWith('//') && !raw.startsWith('/\\') ? raw : null
}));

import { actions } from './+page.server';

interface Cookies {
	get: (name: string) => string | undefined;
	delete: ReturnType<typeof vi.fn>;
}
type Fn = (e: {
	request: Request;
	cookies: Cookies;
	locals: { host: string };
	url: URL;
}) => Promise<unknown>;
const verify = (actions as unknown as { default: Fn }).default;

// returnTo, when given, is the stashed post-verify redirect cookie (#170).
function ev(fields: Record<string, string>, returnTo?: string) {
	const fd = new FormData();
	for (const [k, v] of Object.entries(fields)) fd.set(k, v);
	return {
		request: { formData: async () => fd } as unknown as Request,
		cookies: {
			get: (name: string) => (name === 'return_to' ? returnTo : undefined),
			delete: vi.fn()
		},
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
		expect(setSession).toHaveBeenCalledWith(expect.anything(), 'jwt', 3600, true);
	});

	it('lands back on a stashed valid return path after auto-login (#170)', async () => {
		post.mockResolvedValue({ data: { access_token: 'jwt', expires_in: 3600 }, error: undefined });
		await expect(verify(ev({ token: 'tok' }, '/reserve/abc'))).rejects.toMatchObject({
			status: 303,
			location: '/reserve/abc'
		});
	});

	it('ignores an off-site return cookie, landing on the app (#170)', async () => {
		post.mockResolvedValue({ data: { access_token: 'jwt', expires_in: 3600 }, error: undefined });
		await expect(verify(ev({ token: 'tok' }, 'https://evil.example'))).rejects.toMatchObject({
			status: 303,
			location: '/'
		});
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
