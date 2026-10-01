import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// #397 (ADR-0016): the Settings createToken and revokeToken actions. The token value
// comes back once in the action result; the expiry is an explicit choice; the
// recent-sign-in refusal is surfaced so the page can offer to sign in again.

const post = vi.fn();
const del = vi.fn();
vi.mock('$lib/server/api', () => ({
	backendClient: () => ({ POST: post, DELETE: del })
}));
vi.mock('$lib/server/session', () => ({ setSession: vi.fn() }));

import { actions } from './+page.server';

type ActionFn = (e: {
	request: Request;
	locals: { host: string; token: string };
}) => Promise<unknown>;
const { createToken, revokeToken } = actions as unknown as Record<string, ActionFn>;

function ev(fields: Record<string, string>) {
	const fd = new FormData();
	for (const [k, v] of Object.entries(fields)) fd.set(k, v);
	return {
		request: { formData: async () => fd } as unknown as Request,
		locals: { host: 't.example', token: 'tok' }
	};
}

const created = (name: string) => ({
	data: { token: 'ydg_pat_secretvalue', access_token: { name } },
	error: undefined,
	response: { status: 201 }
});

describe('settings createToken action (ADR-0016)', () => {
	beforeEach(() => {
		post.mockReset();
		vi.useFakeTimers();
		vi.setSystemTime(new Date('2027-06-15T12:00:00Z'));
	});
	afterEach(() => vi.useRealTimers());

	it('creates a token that never expires only when that is chosen', async () => {
		post.mockResolvedValue(created('ci'));
		const res = await createToken(ev({ name: '  ci  ', expiry: 'never' }));
		expect(post).toHaveBeenCalledWith('/api/v1/me/tokens', {
			body: { name: 'ci', never_expires: true }
		});
		expect(res).toEqual({ createdToken: 'ydg_pat_secretvalue', createdTokenName: 'ci' });
	});

	it('turns a number of days into an expiry that many days from now', async () => {
		post.mockResolvedValue(created('backup'));
		await createToken(ev({ name: 'backup', expiry: '30' }));
		expect(post).toHaveBeenCalledWith('/api/v1/me/tokens', {
			body: { name: 'backup', expires_at: '2027-07-15T12:00:00.000Z' }
		});
	});

	it.each([
		['no name', { name: ' ', expiry: 'never' }, 'Give the token a name.'],
		['no expiry choice', { name: 'x', expiry: '' }, 'Choose when the token expires.'],
		['an expiry not offered', { name: 'x', expiry: '7' }, 'Choose when the token expires.']
	])('refuses %s without calling the backend', async (_, fields, message) => {
		const res = (await createToken(ev(fields))) as { status: number; data: { tokenError: string } };
		expect(post).not.toHaveBeenCalled();
		expect(res.status).toBe(400);
		expect(res.data.tokenError).toBe(message);
	});

	it('flags the recent-sign-in refusal so the page can offer to sign in again', async () => {
		post.mockResolvedValue({
			data: undefined,
			error: { detail: 'creating a token needs a recent sign-in; sign in again and retry' },
			response: { status: 403 }
		});
		const res = (await createToken(ev({ name: 'x', expiry: 'never' }))) as {
			status: number;
			data: { tokenError: string; tokenNeedsSignIn: boolean };
		};
		expect(res.status).toBe(403);
		expect(res.data.tokenNeedsSignIn).toBe(true);
		expect(res.data.tokenError).toContain('recent sign-in');
	});

	it('surfaces the 20-token limit as the backend words it', async () => {
		post.mockResolvedValue({
			data: undefined,
			error: { detail: 'this account already has 20 active tokens; revoke one first' },
			response: { status: 409 }
		});
		const res = (await createToken(ev({ name: 'x', expiry: 'never' }))) as {
			status: number;
			data: { tokenError: string; tokenNeedsSignIn: boolean };
		};
		expect(res.status).toBe(409);
		expect(res.data.tokenNeedsSignIn).toBe(false);
		expect(res.data.tokenError).toContain('20 active tokens');
	});
});

describe('settings revokeToken action (ADR-0016)', () => {
	beforeEach(() => del.mockReset());

	it('revokes the token by id', async () => {
		del.mockResolvedValue({ error: undefined });
		const res = await revokeToken(ev({ id: 'tok-1' }));
		expect(del).toHaveBeenCalledWith('/api/v1/me/tokens/{tokenId}', {
			params: { path: { tokenId: 'tok-1' } }
		});
		expect(res).toEqual({ tokenRevoked: true });
	});

	it('reports a failed revoke and a missing id', async () => {
		del.mockResolvedValue({ error: { detail: 'no such token' } });
		const failed = (await revokeToken(ev({ id: 'x' }))) as { data: { tokenError: string } };
		expect(failed.data.tokenError).toBe('Could not revoke the token.');
		del.mockReset();
		const missing = (await revokeToken(ev({}))) as { data: { tokenError: string } };
		expect(del).not.toHaveBeenCalled();
		expect(missing.data.tokenError).toBe('Missing token.');
	});
});
