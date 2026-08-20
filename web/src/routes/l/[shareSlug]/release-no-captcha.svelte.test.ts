import { describe, it, expect, vi } from 'vitest';

// #249: release/withdraw are exempt from the anti-bot check. This proves it at the
// server action — the release path authorizes on the browser's own capability token
// (from its cookie) and never reads a captcha token, so a release with no captcha
// field present must still succeed. A test that only asserted the happy WITH a token
// would not catch a captcha requirement creeping back in; this one submits none.
const state = vi.hoisted(() => ({
	deleteCalled: false,
	deleteHeaders: null as unknown,
	removed: false
}));

vi.mock('$lib/server/api', () => ({
	backendClient: () => ({
		DELETE: async (_path: string, opts: { headers?: unknown }) => {
			state.deleteCalled = true;
			state.deleteHeaders = opts?.headers;
			return { error: undefined, response: { status: 204 } };
		}
	})
}));

// This browser holds a capability for item 'i1' — the cookie the release path reads.
vi.mock('$lib/server/caps', () => ({
	capsForList: () => ({ i1: { reservation_id: 'r1', token: 'cap-tok' } }),
	addCap: () => {},
	removeCap: () => {
		state.removed = true;
	},
	contribCapsForList: () => ({}),
	addContribCap: () => {},
	removeContribCap: () => {}
}));

import { actions } from './+page.server';

type ActionFn = (e: unknown) => Promise<unknown>;

function releaseEvent(fields: Record<string, string>) {
	const fd = new FormData();
	for (const [k, v] of Object.entries(fields)) fd.set(k, v);
	return {
		request: new Request('http://t.example/l/s1?/release', { method: 'POST', body: fd }),
		params: { shareSlug: 's1' },
		locals: { host: 't.example' },
		cookies: { get: () => undefined, set: () => {}, delete: () => {} },
		url: new URL('http://t.example/l/s1')
	};
}

describe('release action is exempt from the anti-bot check (#249)', () => {
	it('releases with no captcha token present, authorizing on the capability cookie', async () => {
		const result = await (actions.release as unknown as ActionFn)(releaseEvent({ item_id: 'i1' }));
		expect(state.deleteCalled).toBe(true);
		// Authorization is the capability token, not a captcha token.
		expect((state.deleteHeaders as Record<string, string>)['X-Capability-Token']).toBe('cap-tok');
		expect(state.removed).toBe(true);
		expect(result).toEqual({ released: true });
	});
});
