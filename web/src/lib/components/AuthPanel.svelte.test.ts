import { describe, it, expect } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/svelte';
import AuthPanelHarness from './AuthPanelHarness.svelte';

describe('AuthPanel', () => {
	it('renders the wordmark, heading, description, form body, and link row', () => {
		render(AuthPanelHarness);
		expect(screen.getByText('YAADEGAR')).toBeInTheDocument();
		expect(screen.getByRole('heading', { name: 'Sign in' })).toBeInTheDocument();
		expect(screen.getByText('Welcome back')).toBeInTheDocument();
		expect(screen.getByTestId('form-body')).toBeInTheDocument();
		expect(screen.getByTestId('forgot-link')).toHaveAttribute('href', '/forgot');
	});

	it('exposes a main landmark so screen-reader users can jump to content', () => {
		render(AuthPanelHarness);
		const main = screen.getByRole('main');
		expect(main).toBeInTheDocument();
		// content lives inside the landmark
		expect(main).toContainElement(screen.getByTestId('form-body'));
	});
});
