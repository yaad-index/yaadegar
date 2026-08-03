import { describe, it, expect, vi, beforeEach } from 'vitest';

// ADR-0011 cut 3: the /forgot action posts the reset request and always reports the
// same neutral "sent" outcome (the backend is enumeration-safe; the UI mirrors it).

const post = vi.fn();
vi.mock('$lib/server/api', () => ({ backendClient: () => ({ POST: post }) }));

import { actions } from './+page.server';

type Fn = (e: { request: Request; locals: { host: string } }) => Promise<unknown>;
const forgot = (actions as unknown as { default: Fn }).default;

function ev(fields: Record<string, string>) {
	const fd = new FormData();
	for (const [k, v] of Object.entries(fields)) fd.set(k, v);
	return {
		request: { formData: async () => fd } as unknown as Request,
		locals: { host: 't.example' }
	};
}

describe('forgot request action (ADR-0011 cut 3)', () => {
	beforeEach(() => post.mockReset());

	it('posts the request and reports sent (enumeration-neutral)', async () => {
		post.mockResolvedValue({ data: undefined, error: undefined, response: { status: 202 } });
		const res = await forgot(ev({ identifier: 'someone' }));
		expect(post).toHaveBeenCalledWith('/api/v1/auth/password-reset/request', {
			body: { identifier: 'someone' }
		});
		expect(res).toEqual({ sent: true });
	});

	it('rejects an empty identifier without calling the backend', async () => {
		const res = (await forgot(ev({ identifier: '  ' }))) as { data: { error: string } };
		expect(post).not.toHaveBeenCalled();
		expect(res.data.error).toContain('username or email');
	});
});
