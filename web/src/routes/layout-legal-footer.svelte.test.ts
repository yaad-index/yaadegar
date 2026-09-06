import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';

// Where the legal footer appears (#302). The footer itself is covered in
// ../lib/components/LegalFooter.svelte.test.ts; what is only true here is the
// placement rule — every page gets it EXCEPT the landing page, whose own footer
// already carries a Legal column (#301).
//
// The route id is read from $app/state, so it is mocked per test rather than taken
// from the shared stub.

const routeState = { id: null as string | null };

vi.mock('$app/state', () => ({
	get page() {
		return { url: new URL('http://test.local/'), route: routeState, form: null, data: {}, status: 200 };
	},
	navigating: {},
	updated: { current: false }
}));

const { default: Layout } = await import('./+layout.svelte');

const children = createRawSnippet(() => ({
	render: () => '<main>page body</main>'
}));

describe('legal footer placement', () => {
	beforeEach(() => {
		routeState.id = null;
	});

	it('renders on a non-landing page', () => {
		routeState.id = '/login';
		render(Layout, { props: { children } });

		expect(screen.getByRole('link', { name: 'Privacy Policy' })).toBeInTheDocument();
		expect(screen.getByRole('link', { name: 'Terms of Service' })).toBeInTheDocument();
	});

	it('does NOT render on the landing page, which has its own Legal column', () => {
		routeState.id = '/';
		render(Layout, { props: { children } });

		expect(screen.queryByRole('link', { name: 'Privacy Policy' })).toBeNull();
		expect(screen.queryByRole('link', { name: 'Terms of Service' })).toBeNull();
	});

	it('renders on a page nobody thought to add it to', () => {
		// The placement excludes one known route rather than listing the routes that
		// opt in, so a route added later is covered without being enumerated. This is
		// the property that stops #302 recurring, and it is the one an opt-in list
		// would quietly lose.
		routeState.id = '/some/route/added/later';
		render(Layout, { props: { children } });

		expect(screen.getByRole('link', { name: 'Privacy Policy' })).toBeInTheDocument();
	});
});
