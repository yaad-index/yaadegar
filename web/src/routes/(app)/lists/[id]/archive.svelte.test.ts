import { describe, it, expect, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, within } from '@testing-library/svelte';
import { page } from '$app/state';
import Page from './+page.svelte';
import type { PageData } from './$types';

// #419 owner surface. The backend keeps an archived item on the OWNER's read —
// archiving is reversible, and an owner who cannot see an archived item cannot put
// it back — so the split between the working list and the finished one happens
// here, and these assertions are what pin it.

const data = (items: unknown[]): PageData =>
	({
		list: {
			id: 'l1',
			title: 'Birthday',
			description: '',
			thank_you_template: '',
			allow_cobuy: true,
			reserver_tier: null,
			visibility: 'private',
			share_slug: 'abc123'
		},
		items,
		registrationEnabled: false,
		noteHtml: {},
		descriptionHtml: '',
		addForm: { id: 'add', valid: true, posted: false, errors: {}, data: {} }
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
	}) as any;

const live = {
	id: 'i1',
	name: 'Still wanted',
	quantity_wanted: 1,
	reserved_quantity: 0,
	availability: 'available',
	priority: 0
};
const archived = {
	id: 'i2',
	name: 'Finished with',
	quantity_wanted: 1,
	reserved_quantity: 1,
	availability: 'reserved',
	priority: 0,
	archived_at: '2026-09-21T18:00:00Z'
};

// section returns the <ul> that follows a heading, so each assertion is scoped to
// one group. A page-wide query would pass just as well if both items rendered in
// the same list, which is the arrangement these tests exist to rule out.
function section(container: HTMLElement, headingText: string): HTMLElement {
	const heading = Array.from(container.querySelectorAll('h2')).find((h) =>
		h.textContent?.includes(headingText)
	);
	if (!heading) throw new Error(`no heading matching ${headingText}`);
	const list = heading.parentElement?.querySelector('ul');
	let el: Element | null = heading;
	while (el && el.tagName !== 'UL') el = el.nextElementSibling;
	return (el ?? list) as HTMLElement;
}

describe('archived items on the owner list page', () => {
	beforeEach(() => {
		// The page chooses its tab from the URL, so a test that does not set one gets
		// the List tab. Cast as the other tests here do: SvelteKit types page.url's
		// pathname as the union of real routes.
		page.url = new URL('http://test.local/lists/l1') as typeof page.url;
	});

	it('keeps archived items out of the working list and in their own group', () => {
		const { container } = render(Page, { props: { data: data([live, archived]), form: null } });

		const working = section(container, 'Your items');
		expect(within(working).getByText('Still wanted')).toBeInTheDocument();
		expect(within(working).queryByText('Finished with')).not.toBeInTheDocument();

		const done = section(container, 'Archived');
		expect(within(done).getByText('Finished with')).toBeInTheDocument();
		expect(within(done).queryByText('Still wanted')).not.toBeInTheDocument();
	});

	it('offers archive on a live item and put-back on an archived one, never the reverse', () => {
		const { container } = render(Page, { props: { data: data([live, archived]), form: null } });

		const working = section(container, 'Your items');
		expect(within(working).getByRole('button', { name: 'Archive' })).toBeInTheDocument();
		expect(within(working).queryByRole('button', { name: 'Put back' })).not.toBeInTheDocument();

		const done = section(container, 'Archived');
		expect(within(done).getByRole('button', { name: 'Put back' })).toBeInTheDocument();
		expect(within(done).queryByRole('button', { name: 'Archive' })).not.toBeInTheDocument();
	});

	it('shows no archived group at all when nothing is archived', () => {
		const { container } = render(Page, { props: { data: data([live]), form: null } });
		const headings = Array.from(container.querySelectorAll('h2')).map((h) => h.textContent ?? '');
		expect(headings.some((h) => h.includes('Archived'))).toBe(false);
	});

	// The empty state has to tell the two cases apart. "No items yet — add one above"
	// is false and misleading on a list whose items are all archived: they are there,
	// they are just finished, and the advice to add one is not the next step.
	it('distinguishes an empty list from a fully archived one', () => {
		render(Page, { props: { data: data([archived]), form: null } });
		expect(screen.getByText('Everything on this list is archived.')).toBeInTheDocument();

		render(Page, { props: { data: data([]), form: null } });
		expect(screen.getByText('No items yet — add one above.')).toBeInTheDocument();
	});

	it('names the archived item in the confirmation, because its row has moved', () => {
		render(Page, {
			props: {
				data: data([archived]),
				// eslint-disable-next-line @typescript-eslint/no-explicit-any
				form: { archived: true, archivedName: 'Finished with', archiveWarnings: [] } as any
			}
		});
		expect(screen.getByRole('status').textContent).toContain('Finished with');
	});

	// Warn, not block (#419 Q2): the archive succeeded, so the warning renders under
	// a success line rather than in place of one. Showing only the warning would read
	// as a refusal, which is the opposite of the decision.
	it('reports a co-buy warning alongside the success, not instead of it', () => {
		render(Page, {
			props: {
				data: data([archived]),
				form: {
					archived: true,
					archivedName: 'Finished with',
					archiveWarnings: ['cobuy_match_pending']
					// eslint-disable-next-line @typescript-eslint/no-explicit-any
				} as any
			}
		});
		const status = screen.getByRole('status');
		expect(status.textContent).toContain('Archived');
		expect(status.textContent).toContain('co-buy');
	});

	it('says nothing about a warning code it has no copy for', () => {
		render(Page, {
			props: {
				data: data([archived]),
				form: {
					archived: true,
					archivedName: 'Finished with',
					archiveWarnings: ['some_future_code']
					// eslint-disable-next-line @typescript-eslint/no-explicit-any
				} as any
			}
		});
		const status = screen.getByRole('status');
		expect(status.textContent).toContain('Archived');
		expect(status.textContent).not.toContain('some_future_code');
	});

	// The omission is disclosed where an owner backing up will read it, not only in
	// the README. An export that quietly drops rows is a bad surprise even when the
	// omission is correct.
	it('says on the export panel that archived items are left out', () => {
		// The export panel lives on the Settings tab, which is where an owner goes to
		// take a backup — so that is where the omission has to be stated.
		page.url = new URL('http://test.local/lists/l1?tab=settings') as typeof page.url;
		render(Page, { props: { data: data([live, archived]), form: null } });
		expect(screen.getByText(/Archived items are not included/)).toBeInTheDocument();
	});
});
