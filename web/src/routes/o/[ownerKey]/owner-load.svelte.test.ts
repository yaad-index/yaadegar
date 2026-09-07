import { describe, it, expect, vi } from 'vitest';

// #308: the owner page's load. Two things matter here — that a missing key stays a
// 404 rather than becoming a server error, and that a null display_name is passed
// through as null rather than being repaired into something. The backend sends null
// precisely when the stored name is the account email, so "helpfully" substituting a
// fallback here would publish the address the backend withheld.

let response: { status: number };
let payload: unknown;

vi.mock('$lib/server/api', () => ({
	backendClient: () => ({
		GET: async () =>
			response.status === 200
				? { data: payload, error: undefined, response }
				: { data: undefined, error: {}, response }
	})
}));

import { load } from './+page.server';

const run = () =>
	(
		load as unknown as (e: {
			params: { ownerKey: string };
			locals: { host: string };
		}) => Promise<{ displayName: string | null; lists: unknown[] }>
	)({ params: { ownerKey: 'k1' }, locals: { host: 't.example' } });

describe('owner page load (#308)', () => {
	it('passes a null display name through untouched', async () => {
		response = { status: 200 };
		payload = { display_name: null, lists: [] };
		const data = await run();
		expect(data.displayName).toBeNull();
	});

	it('passes a real display name through', async () => {
		response = { status: 200 };
		payload = { display_name: 'Wren', lists: [] };
		expect((await run()).displayName).toBe('Wren');
	});

	it('treats a valid key with nothing shared as an empty page, not an error', async () => {
		response = { status: 200 };
		payload = { display_name: 'Wren', lists: [] };
		const data = await run();
		expect(data.lists).toEqual([]);
	});

	it('keeps an unknown key a 404', async () => {
		response = { status: 404 };
		await expect(run()).rejects.toMatchObject({ status: 404 });
	});

	it('does not turn a backend fault into a 404', async () => {
		// A 500 must not be reported to a visitor as "this page does not exist" — that
		// would make an outage indistinguishable from a revoked link.
		response = { status: 500 };
		await expect(run()).rejects.toMatchObject({ status: 500 });
	});
});
