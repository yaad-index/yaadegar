import { describe, it, expect, vi, beforeEach } from 'vitest';

// #185: the Settings updateName action calls PUT /api/v1/me/profile with the typed
// name (blank is passed through so the backend falls it back to the email). On
// success it reports nameSaved (the default enhance invalidation reloads /me so the
// header + field reflect the change); on failure it surfaces the backend's reason.

const put = vi.fn();
vi.mock('$lib/server/api', () => ({
	backendClient: () => ({ PUT: put })
}));

// +page.server also wires the change-password action, which imports session helpers;
// stub the module so importing the actions here doesn't pull the real one.
vi.mock('$lib/server/session', () => ({
	setSession: vi.fn()
}));

import { actions } from './+page.server';

type ActionFn = (e: {
	request: Request;
	locals: { host: string; token: string };
}) => Promise<unknown>;

const updateName = (actions as unknown as { updateName: ActionFn }).updateName;

function ev(fields: Record<string, string>) {
	const fd = new FormData();
	for (const [k, v] of Object.entries(fields)) fd.set(k, v);
	return {
		request: { formData: async () => fd } as unknown as Request,
		locals: { host: 't.example', token: 'tok' }
	};
}

describe('settings updateName action (#185)', () => {
	beforeEach(() => {
		put.mockReset();
	});

	it('sends the name and reports saved on success', async () => {
		put.mockResolvedValue({ data: { name: 'Grace' }, error: undefined, response: { status: 200 } });
		const res = await updateName(ev({ name: 'Grace' }));
		expect(put).toHaveBeenCalledWith('/api/v1/me/profile', { body: { name: 'Grace' } });
		expect(res).toEqual({ nameSaved: true });
	});

	it('passes a blank name through (backend falls it back to email)', async () => {
		put.mockResolvedValue({
			data: { name: 'grace@example.com' },
			error: undefined,
			response: { status: 200 }
		});
		const res = await updateName(ev({ name: '' }));
		expect(put).toHaveBeenCalledWith('/api/v1/me/profile', { body: { name: '' } });
		expect(res).toEqual({ nameSaved: true });
	});

	it('surfaces the backend reason on failure', async () => {
		put.mockResolvedValue({
			data: undefined,
			error: { detail: 'the display name is too long' },
			response: { status: 400 }
		});
		const res = (await updateName(ev({ name: 'x'.repeat(201) }))) as {
			status: number;
			data: { nameError: string };
		};
		expect(res.status).toBe(400);
		expect(res.data.nameError).toBe('the display name is too long');
	});
});
