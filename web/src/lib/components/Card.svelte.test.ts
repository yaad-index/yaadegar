import { describe, it, expect } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/svelte';
import CardHarness from './CardHarness.svelte';

describe('Card', () => {
	it('renders content as a link when href is set', () => {
		render(CardHarness);
		const link = screen.getByRole('link');
		expect(link).toHaveAttribute('href', '/lists/1');
		expect(screen.getByTestId('card-body')).toHaveTextContent('Birthday list');
	});

	it('renders no icon chip when no icon is given', () => {
		const { container } = render(CardHarness);
		expect(screen.queryByTestId('card-icon')).toBeNull();
		expect(container.querySelector('.bg-primary-tint')).toBeNull();
	});

	it('paints the accent chip from the category set when an icon is given', () => {
		const { container } = render(CardHarness, { withIcon: true });
		expect(screen.getByTestId('card-icon')).toBeInTheDocument();
		// rose accent: tint chip + primary foreground, straight from the set.
		expect(container.querySelector('.bg-primary-tint.text-primary')).not.toBeNull();
	});
});
