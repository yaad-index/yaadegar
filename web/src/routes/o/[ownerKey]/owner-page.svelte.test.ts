import { describe, it, expect } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/svelte';
import OwnerPage from './+page.svelte';

// #308: the giver-facing index of one owner's shared lists.

const row = (title: string, slug: string, count = 2) => ({
	title,
	share_slug: slug,
	item_count: count,
	item_previews: []
});

const renderPage = (data: { displayName: string | null; lists: ReturnType<typeof row>[] }) =>
	render(OwnerPage, { data } as never);

describe('owner page (#308)', () => {
	it('names the owner when they chose a display name', () => {
		renderPage({ displayName: 'Wren', lists: [row('Birthday', 's-birthday')] });
		expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent("Wren's lists");
	});

	it('stays unattributed when there is no display name of their own', () => {
		// The backend sends null precisely when the stored name is the account email,
		// so "no name" must render as a neutral heading rather than as anything derived
		// from the account.
		renderPage({ displayName: null, lists: [row('Birthday', 's-birthday')] });
		expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Shared lists');
	});

	it("renders one row per list, linking through to that list's own public view", () => {
		renderPage({
			displayName: 'Wren',
			lists: [row('Birthday', 's-birthday', 3), row('Housewarming', 's-house', 1)]
		});

		const first = screen.getByRole('link', { name: /Birthday/ });
		expect(first).toHaveAttribute('href', expect.stringContaining('s-birthday'));
		expect(screen.getByRole('link', { name: /Housewarming/ })).toHaveAttribute(
			'href',
			expect.stringContaining('s-house')
		);
		expect(screen.getByText('3 items')).toBeInTheDocument();
		expect(screen.getByText('1 items')).toBeInTheDocument();
	});

	it('offers no way to reserve — every row is a link onward, and there are no buttons', () => {
		// The page is read-only by design: giver flows live on each list's own view.
		renderPage({ displayName: 'Wren', lists: [row('Birthday', 's-birthday')] });
		expect(screen.queryAllByRole('button')).toHaveLength(0);
	});

	it('says the page is empty rather than looking broken when nothing is shared', () => {
		// A valid key with nothing shared is a 200 with no rows; a wrong key is a 404.
		// The owner checking their own link has to be able to tell those apart, so the
		// empty state names the situation instead of rendering as a bare page.
		renderPage({ displayName: 'Wren', lists: [] });
		expect(screen.getByText('Nothing shared yet')).toBeInTheDocument();
		expect(screen.queryAllByRole('listitem')).toHaveLength(0);
	});
});
