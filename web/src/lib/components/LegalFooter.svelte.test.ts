import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import LegalFooter from './LegalFooter.svelte';

// The footer that makes /privacy and /terms reachable on an instance that never
// renders the landing page (#302). What matters is that both documents are linked,
// that the links are in-app routes rather than anything that could 404, and that
// they are named the same as in the landing footer's Legal column.

describe('LegalFooter', () => {
	it('links both legal documents', () => {
		render(LegalFooter);

		const privacy = screen.getByRole('link', { name: 'Privacy Policy' });
		const terms = screen.getByRole('link', { name: 'Terms of Service' });

		expect(privacy).toHaveAttribute('href', '/privacy');
		expect(terms).toHaveAttribute('href', '/terms');
	});

	it('names the documents exactly as the landing footer does', () => {
		// Reaching the same page from two places must not present it under two
		// different names.
		render(LegalFooter);
		expect(screen.getByText('Privacy Policy')).toBeInTheDocument();
		expect(screen.getByText('Terms of Service')).toBeInTheDocument();
	});

	it('exposes the links as a labelled navigation landmark', () => {
		// A footer of links is navigation; labelling it keeps it distinguishable from
		// the page's primary nav for anyone moving by landmark.
		render(LegalFooter);
		expect(screen.getByRole('navigation', { name: 'Legal' })).toBeInTheDocument();
	});

	it('keeps both links in the same tab', () => {
		// These are this instance's own routes, unlike the landing footer's project
		// columns, which deliberately open externally.
		render(LegalFooter);
		for (const name of ['Privacy Policy', 'Terms of Service']) {
			const link = screen.getByRole('link', { name });
			expect(link).not.toHaveAttribute('target');
			expect(link).not.toHaveAttribute('rel');
		}
	});
});
