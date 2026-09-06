import { describe, it, expect } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/svelte';
import LegalDocument from './LegalDocument.svelte';
import { resolveLegalPage } from '$lib/legal';

// The rendering half of #300. resolveLegalPage's own branches are unit-tested in
// ../legal.test.ts; this asserts what a visitor actually sees on the two states that
// matter — an installation that published nothing, and one that published text.

describe('LegalDocument — unconfigured (the placeholder)', () => {
	it('shows the PLACE HOLDER marker and says nothing has been published', () => {
		render(LegalDocument, { props: { page: resolveLegalPage('privacy', {}) } });
		expect(screen.getByTestId('legal-placeholder')).toHaveTextContent('PLACE HOLDER');
		expect(screen.getByRole('heading', { name: 'Privacy Policy' })).toBeInTheDocument();
		expect(screen.getByText(/has not published a privacy policy/)).toBeInTheDocument();
	});

	it('names the env key the operator sets, so the fix is on the page that shows the problem', () => {
		render(LegalDocument, { props: { page: resolveLegalPage('terms', {}) } });
		expect(screen.getByText('YAADEGAR_TERMS_TEXT')).toBeInTheDocument();
		expect(screen.getByText(/has not published terms of service/)).toBeInTheDocument();
	});
});

describe('LegalDocument — configured', () => {
	it('renders each paragraph of the installation text and drops the placeholder', () => {
		const page = resolveLegalPage('privacy', {
			YAADEGAR_PRIVACY_TEXT: 'First paragraph.\n\nSecond paragraph.'
		});
		render(LegalDocument, { props: { page } });
		expect(screen.getByText('First paragraph.')).toBeInTheDocument();
		expect(screen.getByText('Second paragraph.')).toBeInTheDocument();
		// The placeholder must be GONE, not merely visually secondary: a page that shows a
		// published policy and the words PLACE HOLDER together is worse than either alone.
		expect(screen.queryByTestId('legal-placeholder')).not.toBeInTheDocument();
		expect(screen.queryByText('PLACE HOLDER')).not.toBeInTheDocument();
	});

	it('renders operator text as TEXT, never as markup', () => {
		const page = resolveLegalPage('terms', {
			YAADEGAR_TERMS_TEXT: 'Contact <b>us</b> <script>alert(1)</script> anytime.'
		});
		const { container } = render(LegalDocument, { props: { page } });
		// The angle brackets survive as characters, and neither tag became an element —
		// the assertion that fails the moment someone reaches for {@html}.
		expect(
			screen.getByText('Contact <b>us</b> <script>alert(1)</script> anytime.')
		).toBeInTheDocument();
		expect(container.querySelector('b')).toBeNull();
		expect(container.querySelector('script')).toBeNull();
	});
});
