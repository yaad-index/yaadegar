import { describe, it, expect } from 'vitest';
import { load } from './+page.server';

// The site-root loader is the one piece of genuinely new routing in the landing PR
// (#236): an authenticated visitor is redirected on to the app, a signed-out visitor
// gets the marketing page. This asserts ONLY that single two-way branch — a regression
// here would silently send every logged-in arrival to the marketing page. The
// owner-vs-giver split is downstream in (app)/lists/+page.server.ts and is covered by
// its own test (dashboard.svelte.test.ts); re-asserting a giver role here would both
// duplicate that and imply role logic this loader does not contain.
describe('landing root redirect (#236)', () => {
	it('redirects an authenticated visitor on to the dashboard at /lists', () => {
		let thrown: { status?: number; location?: string } | undefined;
		try {
			// SvelteKit's redirect() throws; the loader is synchronous, so this throws here.
			load({ locals: { token: 'tok', host: 't.example' } } as never);
		} catch (e) {
			thrown = e as typeof thrown;
		}
		expect(thrown?.status).toBe(303);
		expect(thrown?.location).toBe('/lists');
	});

	it('renders the landing (no redirect) for a signed-out visitor', () => {
		const res = load({ locals: { host: 't.example' } } as never);
		expect(res).toEqual({});
	});
});
