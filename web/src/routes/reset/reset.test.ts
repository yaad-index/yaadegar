import { describe, it, expect, vi, beforeEach } from 'vitest';

// ADR-0011 cut 3: the /reset confirm action posts the token + new password, and on
// success auto-logs-in (sets the session cookie, redirects to the app). It surfaces
// the backend's real reason on failure and catches a confirm mismatch client-side.

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
const reset = (actions as unknown as { default: Fn }).default;

function ev(fields: Record<string, string>) {
	const fd = new FormData();
	for (const [k, v] of Object.entries(fields)) fd.set(k, v);
	return {
		request: { formData: async () => fd } as unknown as Request,
		cookies: {},
		locals: { host: 't.example' },
		url: new URL('https://t.example/reset')
	};
}

describe('reset confirm action (ADR-0011 cut 3)', () => {
	beforeEach(() => {
		post.mockReset();
		setSession.mockReset();
	});

	it('on success sets the session cookie and redirects to the app (auto-login)', async () => {
		post.mockResolvedValue({ data: { access_token: 'jwt', expires_in: 3600 }, error: undefined });
		await expect(
			reset(ev({ token: 'tok', new_password: 'new-password', confirm_password: 'new-password' }))
		).rejects.toMatchObject({ status: 303, location: '/' });
		expect(setSession).toHaveBeenCalledWith({}, 'jwt', 3600, true);
	});

	it('surfaces the backend reason on an invalid/expired token', async () => {
		post.mockResolvedValue({
			data: undefined,
			error: { detail: 'this reset link is invalid or has expired' }
		});
		const res = (await reset(
			ev({ token: 'bad', new_password: 'new-password', confirm_password: 'new-password' })
		)) as { data: { error: string } };
		expect(setSession).not.toHaveBeenCalled();
		expect(res.data.error).toContain('invalid or has expired');
	});

	it('rejects a mismatch without calling the backend', async () => {
		const res = (await reset(
			ev({ token: 'tok', new_password: 'new-password', confirm_password: 'nope' })
		)) as { data: { error: string } };
		expect(post).not.toHaveBeenCalled();
		expect(res.data.error).toContain('do not match');
	});
});
