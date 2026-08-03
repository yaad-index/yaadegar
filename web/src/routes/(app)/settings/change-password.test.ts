import { describe, it, expect, vi, beforeEach } from 'vitest';

// ADR-0011 cut 2: the Settings changePassword action calls PUT /api/v1/me/password,
// and on success swaps the session cookie to the re-issued acting-session token (so
// the owner stays logged in here). On failure it surfaces the backend's real reason
// (#144), and a confirm mismatch is caught client-side before any backend call.

const put = vi.fn();
vi.mock('$lib/server/api', () => ({
	backendClient: () => ({ PUT: put })
}));

const setSession = vi.fn();
vi.mock('$lib/server/session', () => ({
	setSession: (...args: unknown[]) => setSession(...args)
}));

import { actions } from './+page.server';

type ActionFn = (e: {
	request: Request;
	cookies: Record<string, unknown>;
	locals: { host: string; token: string };
	url: URL;
}) => Promise<unknown>;

const changePassword = (actions as unknown as { changePassword: ActionFn }).changePassword;

function ev(fields: Record<string, string>) {
	const fd = new FormData();
	for (const [k, v] of Object.entries(fields)) fd.set(k, v);
	return {
		request: { formData: async () => fd } as unknown as Request,
		cookies: {},
		locals: { host: 't.example', token: 'tok' },
		url: new URL('https://t.example/settings')
	};
}

describe('settings changePassword action (ADR-0011 cut 2)', () => {
	beforeEach(() => {
		put.mockReset();
		setSession.mockReset();
	});

	it('on success swaps the session cookie and reports changed', async () => {
		put.mockResolvedValue({
			data: { access_token: 'new-jwt', expires_in: 3600 },
			error: undefined,
			response: { status: 200 }
		});
		const res = await changePassword(
			ev({
				current_password: 'old-pass',
				new_password: 'new-password',
				confirm_password: 'new-password'
			})
		);
		expect(put).toHaveBeenCalledWith('/api/v1/me/password', {
			body: { current_password: 'old-pass', new_password: 'new-password' }
		});
		// setSession(cookies, token, maxAge, secure) — https url → secure true.
		expect(setSession).toHaveBeenCalledWith({}, 'new-jwt', 3600, true);
		expect(res).toEqual({ passwordChanged: true });
	});

	it('surfaces the backend real reason on a wrong current password', async () => {
		put.mockResolvedValue({
			data: undefined,
			error: { detail: 'the current password is incorrect' },
			response: { status: 403 }
		});
		const res = (await changePassword(
			ev({
				current_password: 'wrong',
				new_password: 'new-password',
				confirm_password: 'new-password'
			})
		)) as { status: number; data: { passwordError: string } };
		expect(setSession).not.toHaveBeenCalled();
		expect(res.status).toBe(403);
		expect(res.data.passwordError).toBe('the current password is incorrect');
	});

	it('rejects a confirm mismatch without calling the backend', async () => {
		const res = (await changePassword(
			ev({
				current_password: 'old-pass',
				new_password: 'new-password',
				confirm_password: 'different'
			})
		)) as { data: { passwordError: string } };
		expect(put).not.toHaveBeenCalled();
		expect(setSession).not.toHaveBeenCalled();
		expect(res.data.passwordError).toContain('do not match');
	});

	it('rejects an empty field without calling the backend', async () => {
		const res = (await changePassword(
			ev({ current_password: '', new_password: 'new-password', confirm_password: 'new-password' })
		)) as { data: { passwordError: string } };
		expect(put).not.toHaveBeenCalled();
		expect(res.data.passwordError).toContain('current and new password');
	});
});
