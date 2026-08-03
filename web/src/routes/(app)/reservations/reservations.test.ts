import { describe, it, expect, vi, beforeEach } from 'vitest';

const get = vi.fn();
const del = vi.fn();
vi.mock('$lib/server/api', () => ({
	backendClient: () => ({ GET: get, DELETE: del })
}));

import { load, actions } from './+page.server';

const locals = { host: 't.example', token: 'tok' };

function formRequest(fields: Record<string, string>): Request {
	const fd = new FormData();
	for (const [k, v] of Object.entries(fields)) fd.set(k, v);
	return new Request('http://t.example/reservations?/release', { method: 'POST', body: fd });
}

describe('reserver dashboard (#20 / cut 3b)', () => {
	beforeEach(() => {
		get.mockReset();
		del.mockReset();
	});

	it('load returns the account reservations', async () => {
		get.mockResolvedValue({
			data: {
				items: [{ reservation_id: 'r1', item_name: 'Boardgame', list_title: 'L', share_slug: 's' }],
				total: 1
			},
			error: undefined,
			response: { status: 200 }
		});
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const res = (await load({ locals } as any)) as { reservations: { item_name: string }[] };
		expect(res.reservations).toHaveLength(1);
		expect(res.reservations[0].item_name).toBe('Boardgame');
	});

	it('load tolerates an empty/failed read', async () => {
		get.mockResolvedValue({ data: undefined, error: { detail: 'x' }, response: { status: 500 } });
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const res = (await load({ locals } as any)) as { reservations: unknown[] };
		expect(res.reservations).toEqual([]);
	});

	it('release calls DELETE and reports success', async () => {
		del.mockResolvedValue({ error: undefined, response: { status: 204 } });
		const res = await actions.release({
			request: formRequest({ reservation_id: 'r1' }),
			locals
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
		} as any);
		expect(del).toHaveBeenCalledWith('/api/v1/me/reservations/{reservationId}', {
			params: { path: { reservationId: 'r1' } }
		});
		expect(res).toEqual({ released: true });
	});

	it('release surfaces a backend failure as a 400', async () => {
		del.mockResolvedValue({ error: { detail: 'nope' }, response: { status: 404 } });
		const res = await actions.release({
			request: formRequest({ reservation_id: 'r1' }),
			locals
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
		} as any);
		// fail(400, ...) returns an ActionFailure with status 400.
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		expect((res as any).status).toBe(400);
	});
});
