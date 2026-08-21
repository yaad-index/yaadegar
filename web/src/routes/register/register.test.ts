import { describe, it, expect, vi, beforeEach } from 'vitest';

// ADR-0012 cut 1a: the /register action posts email+password (captcha_token empty),
// shows the same neutral "check your email" message on any success (enumeration-
// neutral), a distinct message when registration is disabled (403), surfaces the
// backend reason on other failures, and catches a confirm mismatch client-side.

const post = vi.fn();
const get = vi.fn();
vi.mock('$lib/server/api', () => ({ backendClient: () => ({ POST: post, GET: get }) }));
// Resolve the real returnTo module (safeReturnTo + returnToCookie) under its $lib
// specifier, which vitest can't alias-resolve for a route test on its own. Using the
// real module — not a hand-copied stub — so the test exercises the actual cookie
// owner and can't drift from it (the whole point of #243).
vi.mock('$lib/server/returnTo', async () => await import('../../lib/server/returnTo'));
import { actions, load } from './+page.server';

interface Cookies {
	set: ReturnType<typeof vi.fn>;
}
type Fn = (e: {
	request: Request;
	locals: { host: string };
	cookies: Cookies;
	url: URL;
}) => Promise<unknown>;
const register = (actions as unknown as { default: Fn }).default;

// url carries the ?return_to= the register form echoes back (#170); cookies.set
// captures the return-path stash for the post-verify redirect.
function ev(fields: Record<string, string>, returnTo?: string) {
	const fd = new FormData();
	for (const [k, v] of Object.entries(fields)) fd.set(k, v);
	const url = new URL('http://t.example/register');
	if (returnTo) url.searchParams.set('return_to', returnTo);
	return {
		request: { formData: async () => fd } as unknown as Request,
		locals: { host: 't.example' },
		cookies: { set: vi.fn() },
		url
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

	it('stashes a valid return path in a cookie on success (#170)', async () => {
		post.mockResolvedValue({ data: undefined, error: undefined, response: { status: 202 } });
		const e = ev(
			{
				email: 'newbie@example.com',
				password: 'long-enough-pass',
				confirm_password: 'long-enough-pass'
			},
			'/reserve/abc'
		);
		const res = (await register(e)) as { sent: boolean };
		expect(res).toEqual({ sent: true });
		expect(e.cookies.set).toHaveBeenCalledWith('return_to', '/reserve/abc', expect.anything());
	});

	it('does not stash an off-site return path (#170)', async () => {
		post.mockResolvedValue({ data: undefined, error: undefined, response: { status: 202 } });
		const e = ev(
			{
				email: 'newbie@example.com',
				password: 'long-enough-pass',
				confirm_password: 'long-enough-pass'
			},
			'//evil.example/phish'
		);
		await register(e);
		expect(e.cookies.set).not.toHaveBeenCalled();
	});
});

// #253: the loader reads the instance's registration_enabled (the same signal the login
// page uses to hide its "Sign up" entry point) so the page can render a not-enabled state
// up front instead of a form that can only 403.
type LoadFn = (e: {
	locals: { host: string };
	url: URL;
}) => Promise<{ registrationEnabled: boolean; returnTo: string }>;
const loadRegister = load as unknown as LoadFn;

function loadEv(returnTo?: string) {
	const url = new URL('http://t.example/register');
	if (returnTo) url.searchParams.set('return_to', returnTo);
	return { locals: { host: 't.example' }, url };
}

describe('register load (#253)', () => {
	beforeEach(() => {
		get.mockReset();
	});

	it('reports registration disabled when the instance policy is off', async () => {
		get.mockResolvedValue({ data: { registration_enabled: false } });
		const res = await loadRegister(loadEv());
		expect(get).toHaveBeenCalledWith('/api/v1/auth/methods');
		expect(res.registrationEnabled).toBe(false);
	});

	it('reports registration enabled when the instance policy is on', async () => {
		get.mockResolvedValue({ data: { registration_enabled: true } });
		const res = await loadRegister(loadEv());
		expect(res.registrationEnabled).toBe(true);
	});

	it('defaults to disabled when the backend cannot be reached', async () => {
		get.mockResolvedValue({ data: undefined });
		const res = await loadRegister(loadEv());
		expect(res.registrationEnabled).toBe(false);
	});

	it('still carries a valid return path through (#170)', async () => {
		get.mockResolvedValue({ data: { registration_enabled: true } });
		const res = await loadRegister(loadEv('/reserve/abc'));
		expect(res.returnTo).toBe('/reserve/abc');
	});
});
