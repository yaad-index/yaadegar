import { describe, it, expect } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/svelte';
import PageShellHarness from './PageShellHarness.svelte';

describe('PageShell', () => {
	it('renders the wordmark and the content by default', () => {
		render(PageShellHarness);
		expect(screen.getByRole('link', { name: 'Yaadegar' })).toBeInTheDocument();
		expect(screen.getByTestId('content')).toBeInTheDocument();
		expect(screen.queryByTestId('empty')).toBeNull();
	});

	it('shows the empty state in place of the content when isEmpty is set', () => {
		render(PageShellHarness, { isEmpty: true });
		expect(screen.getByTestId('empty')).toBeInTheDocument();
		expect(screen.queryByTestId('content')).toBeNull();
	});

	it('owns exactly one main landmark (the shell contract, #205)', () => {
		const { container } = render(PageShellHarness);
		expect(container.querySelectorAll('main')).toHaveLength(1);
		expect(screen.getByRole('main')).toContainElement(screen.getByTestId('content'));
	});
});
