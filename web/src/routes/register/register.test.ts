import { describe, it, expect, vi, beforeEach } from 'vitest';

// ADR-0012 cut 1a: the /register action posts email+password (captcha_token empty),
// shows the same neutral "check your email" message on any success (enumeration-
// neutral), a distinct message when registration is disabled (403), surfaces the
// backend reason on other failures, and catches a confirm mismatch client-side.

const post = vi.fn();
vi.mock('$lib/server/api', () => ({ backendClient: () => ({ POST: post }) }));

import { actions } from './+page.server';

type Fn = (e: { request: Request; locals: { host: string } }) => Promise<unknown>;
const register = (actions as unknown as { default: Fn }).default;

function ev(fields: Record<string, string>) {
	const fd = new FormData();
	for (const [k, v] of Object.entries(fields)) fd.set(k, v);
	return {
		request: { formData: async () => fd } as unknown as Request,
		locals: { host: 't.example' }
	};
}

describe('register action (ADR-0012 cut 1a)', () => {
	beforeEach(() => {
		post.mockReset();
	});

	it('on success shows the neutral check-your-email message and does not auto-login', async () => {
		post.mockResolvedValue({ data: undefined, error: undefined, response: { status: 202 } });
		const res = (await register(
			ev({
				email: 'newbie@example.com',
				password: 'long-enough-pass',
				confirm_password: 'long-enough-pass'
			})
		)) as { sent: boolean };
		expect(post).toHaveBeenCalledWith('/api/v1/auth/register', {
			body: { email: 'newbie@example.com', password: 'long-enough-pass', captcha_token: '' }
		});
		expect(res).toEqual({ sent: true });
	});

	it('shows a distinct message when registration is disabled (403)', async () => {
		post.mockResolvedValue({
			data: undefined,
			error: { detail: 'self-registration is not enabled on this instance' },
			response: { status: 403 }
		});
		const res = (await register(
			ev({
				email: 'newbie@example.com',
				password: 'long-enough-pass',
				confirm_password: 'long-enough-pass'
			})
		)) as { status: number; data: { disabled: boolean } };
		expect(res.status).toBe(403);
		expect(res.data.disabled).toBe(true);
	});

	it('surfaces the backend reason on another failure (e.g. too-short password)', async () => {
		post.mockResolvedValue({
			data: undefined,
			error: { detail: 'the password must be at least 8 characters' },
			response: { status: 400 }
		});
		const res = (await register(
			ev({ email: 'newbie@example.com', password: 'short', confirm_password: 'short' })
		)) as { status: number; data: { error: string } };
		expect(res.status).toBe(400);
		expect(res.data.error).toContain('at least 8 characters');
	});

	it('rejects a confirm mismatch without calling the backend', async () => {
		const res = (await register(
			ev({ email: 'newbie@example.com', password: 'long-enough-pass', confirm_password: 'nope' })
		)) as { data: { error: string } };
		expect(post).not.toHaveBeenCalled();
		expect(res.data.error).toContain('do not match');
	});
});
