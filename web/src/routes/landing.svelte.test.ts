import { describe, it, expect, afterEach } from 'vitest';
import { env } from '$env/dynamic/private';
import { load } from './+page.server';

// Authenticated routing is unchanged across every root-page mode, so it is asserted once
// here; the signed-out branch is what YAADEGAR_ROOT_PAGE governs (#256, ADR-0015). The
// owner-vs-giver split is downstream in (app)/lists/+page.server.ts, covered by its own
// test (dashboard.svelte.test.ts); re-asserting a giver role here would duplicate that
// and imply role logic this loader does not contain. resolveRootPage's own branches and
// the CTA-href allowlist are unit-tested in ../lib/server/rootPage.test.ts; this exercises
// the loader wiring on top of it.
function loadSignedOut() {
	return load({ locals: { host: 't.example' } } as never);
}

describe('landing root loader (#236/#256)', () => {
	afterEach(() => {
		for (const k of Object.keys(env)) delete env[k];
	});

	it('redirects an authenticated visitor on to the dashboard at /lists, in any mode', () => {
		env.YAADEGAR_ROOT_PAGE = 'login'; // even the login mode must not intercept an authed visitor
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

	it('renders the bundled landing (no redirect) for a signed-out visitor when unset', () => {
		expect(loadSignedOut()).toEqual({});
	});

	it('redirects a signed-out visitor to /login when YAADEGAR_ROOT_PAGE=login', () => {
		env.YAADEGAR_ROOT_PAGE = 'login';
		let thrown: { status?: number; location?: string } | undefined;
		try {
			loadSignedOut();
		} catch (e) {
			thrown = e as typeof thrown;
		}
		expect(thrown?.status).toBe(303);
		expect(thrown?.location).toBe('/login');
	});

	it('returns the operator strings for a signed-out visitor in custom mode', () => {
		env.YAADEGAR_ROOT_PAGE = 'custom';
		env.YAADEGAR_ROOT_HEADLINE = 'Our family wishlist';
		expect(loadSignedOut()).toMatchObject({ custom: { headline: 'Our family wishlist' } });
	});
});
