import { describe, it, expect, vi } from 'vitest';

// ADR-0013: the reserve action forwards the widget's captcha_token to the backend
// when present, and omits it entirely when the giver had no widget/token. The
// backendClient POST is captured so the body can be inspected; the response is a
// 202 pending_confirmation so the action returns without touching the caps cookie.
const state = vi.hoisted(() => ({ body: null as unknown, next: null as unknown }));
vi.mock('$lib/server/api', () => ({
	backendClient: () => ({
		POST: async (_path: string, opts: { body?: unknown }) => {
			state.body = opts?.body;
			return state.next;
		}
	})
}));

import { actions } from './+page.server';

const ev = (name: string, fields: Record<string, string>) => {
	const fd = new FormData();
	for (const [k, v] of Object.entries(fields)) fd.set(k, v);
	return {
		request: new Request('http://t.example/l/s1?/' + name, { method: 'POST', body: fd }),
		params: { shareSlug: 's1' },
		locals: { host: 't.example' },
		cookies: { get: () => undefined, set: () => {}, delete: () => {} },
		url: new URL('http://t.example/l/s1')
	};
};

type ActionFn = (e: ReturnType<typeof ev>) => Promise<unknown>;
const callReserve = (fields: Record<string, string>) => {
	// A pending_confirmation success: body is captured, no capability-token/caps write.
	state.next = {
		data: { reservation_id: 'r1', status: 'pending_confirmation' },
		error: undefined,
		response: { status: 202 }
	};
	return (actions.reserve as unknown as ActionFn)(ev('reserve', fields));
};

describe('reserve action captcha_token forwarding (ADR-0013)', () => {
	it('includes captcha_token in the POST body when the widget provided one', async () => {
		await callReserve({ item_id: 'i1', giver_email: 'g@example.com', captcha_token: 'tok-xyz' });
		expect((state.body as { captcha_token?: string }).captcha_token).toBe('tok-xyz');
	});

	it('omits captcha_token entirely when none was provided', async () => {
		await callReserve({ item_id: 'i1', giver_email: 'g@example.com' });
		expect(state.body as Record<string, unknown>).not.toHaveProperty('captcha_token');
	});

	it('omits captcha_token when the field is present but empty', async () => {
		await callReserve({ item_id: 'i1', giver_email: 'g@example.com', captcha_token: '' });
		expect(state.body as Record<string, unknown>).not.toHaveProperty('captcha_token');
	});
});
