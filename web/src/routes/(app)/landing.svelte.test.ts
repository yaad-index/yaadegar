import { describe, it, expect, vi, beforeEach } from 'vitest';

const get = vi.fn();
vi.mock('$lib/server/api', () => ({
	backendClient: () => ({ GET: get })
}));

import { load } from './+page.server';

const locals = { host: 't.example', token: 'tok' };

describe('owner-root landing (#20 / cut 3b)', () => {
	beforeEach(() => get.mockReset());

	it('redirects a giver to their reserver dashboard', async () => {
		const parent = async () => ({ user: { role: 'giver' } });
		// SvelteKit redirect() throws a Redirect (status + location).
		await expect(
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			load({ locals, parent } as any)
		).rejects.toMatchObject({ status: 303, location: '/reservations' });
		expect(get).not.toHaveBeenCalled();
	});

	it('an owner stays on "your lists"', async () => {
		get.mockResolvedValue({
			data: { items: [{ id: 'l1', title: 'L' }] },
			error: undefined,
			response: { status: 200 }
		});
		const parent = async () => ({ user: { role: 'owner' } });
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const res = (await load({ locals, parent } as any)) as { lists: unknown[] };
		expect(res.lists).toHaveLength(1);
		expect(get).toHaveBeenCalledWith('/api/v1/lists', { params: { query: { limit: 200 } } });
	});
});
