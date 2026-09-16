import { describe, it, expect, vi, beforeEach } from 'vitest';

// #406: the two web surfaces disagreed about what a title is. Creation's schema was
// a bare `.min(1)`, which passes on "   ", so a list could be created with a
// whitespace-only title and then refused by the settings form on the next save.
//
// The rule now lives in the backend, and this surface restates it only for the fast
// local message. What is worth asserting is the half that was wrong — that a
// whitespace-only title no longer reaches the API at all, and that a padded one is
// sent trimmed, so the value posted here matches the value the backend would store.

let posted: Record<string, unknown> | undefined;
let postCalls = 0;

vi.mock('$lib/server/api', () => ({
	backendClient: () => ({
		GET: async () => ({ data: { items: [] } }),
		POST: async (_path: string, opts: { body: Record<string, unknown> }) => {
			postCalls += 1;
			posted = opts.body;
			return { error: undefined };
		}
	})
}));

import { actions } from './+page.server';

// The return type is annotated rather than inferred: `posted` is only ever assigned
// from inside the hoisted vi.mock factory, which TypeScript's control flow does not
// see, so it would otherwise narrow to `undefined` and make every field access an
// error on `never` (same reason as the sibling settings test).
type Created = { body: Record<string, unknown> | undefined; calls: number; result: unknown };

async function createList(title: string): Promise<Created> {
	posted = undefined;
	postCalls = 0;
	const fd = new FormData();
	fd.append('title', title);
	const create = actions.create as unknown as (e: {
		request: Request;
		locals: { host: string; token: string };
	}) => Promise<unknown>;
	const result = await create({
		request: new Request('http://t.example/lists?/create', { method: 'POST', body: fd }),
		locals: { host: 't.example', token: 'tok' }
	});
	return { body: posted, calls: postCalls, result };
}

describe('list creation title validation (#406)', () => {
	beforeEach(() => {
		posted = undefined;
		postCalls = 0;
	});

	it('refuses a whitespace-only title instead of creating one', async () => {
		const { calls, result } = await createList('   ');
		// The defect was that this reached the API and succeeded. The request not
		// being made is the assertion; the 400 is how it is reported.
		expect(calls).toBe(0);
		expect((result as { status?: number }).status).toBe(400);
	});

	it('refuses an empty title', async () => {
		const { calls } = await createList('');
		expect(calls).toBe(0);
	});

	it('sends a padded title trimmed, matching what the backend stores', async () => {
		const { body, calls } = await createList('  Wedding gifts  ');
		expect(calls).toBe(1);
		expect(body?.title).toBe('Wedding gifts');
	});

	it('still creates an ordinary title', async () => {
		const { body, calls } = await createList('Wedding gifts');
		expect(calls).toBe(1);
		expect(body?.title).toBe('Wedding gifts');
	});
});
