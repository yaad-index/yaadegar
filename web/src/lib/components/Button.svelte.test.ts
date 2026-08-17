import { describe, it, expect } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/svelte';
import ButtonHarness from './ButtonHarness.svelte';

describe('Button', () => {
	it('renders its content and defaults to type=button', () => {
		render(ButtonHarness);
		const button = screen.getByRole('button', { name: 'Save' });
		expect(button).toHaveAttribute('type', 'button');
	});

	it('carries the primary fill by default and the bordered treatment for secondary', () => {
		const { rerender } = render(ButtonHarness);
		expect(screen.getByRole('button', { name: 'Save' })).toHaveClass('bg-primary');

		rerender({ variant: 'secondary' });
		const button = screen.getByRole('button', { name: 'Save' });
		expect(button).not.toHaveClass('bg-primary');
		expect(button).toHaveClass('border');
	});

	it('passes native attributes through (type, disabled)', () => {
		render(ButtonHarness, { type: 'submit', disabled: true });
		const button = screen.getByRole('button', { name: 'Save' });
		expect(button).toHaveAttribute('type', 'submit');
		expect(button).toBeDisabled();
	});
});
