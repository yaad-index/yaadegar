import { describe, it, expect } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/svelte';
import Field from './Field.svelte';

// The field's job beyond looking right is that the error is *announced*, not just
// coloured, and that the label actually points at the input. These check the
// wiring a visual review can't see.
describe('Field', () => {
	it('associates the label with the input', () => {
		render(Field, { label: 'Username', name: 'username' });
		const input = screen.getByLabelText('Username');
		expect(input).toBeInTheDocument();
		expect(input).toHaveAttribute('name', 'username');
	});

	it('announces the error and marks the input invalid', () => {
		render(Field, { label: 'Email', error: 'Enter a valid email' });
		const alert = screen.getByRole('alert');
		expect(alert).toHaveTextContent('Enter a valid email');

		const input = screen.getByLabelText('Email');
		expect(input).toHaveAttribute('aria-invalid', 'true');
		expect(input).toHaveAttribute('aria-describedby', alert.id);
	});

	it('gives an errored input a visible focus indicator, not just a red border', () => {
		// Regression guard (WCAG 2.4.7): with focus:outline-none, an error state
		// whose focus style only re-asserts the resting red border is visually
		// inert on focus. The focus indicator must be a ring that appears on focus.
		render(Field, { label: 'Email', error: 'Required' });
		const input = screen.getByLabelText('Email');
		expect(input.className).toContain('focus-visible:ring-2');
		expect(input.className).toContain('focus-visible:ring-red-500');
	});

	it('shows the hint only while there is no error', () => {
		const { rerender } = render(Field, { label: 'Name', hint: 'As it appears on the gift' });
		expect(screen.getByText('As it appears on the gift')).toBeInTheDocument();
		expect(screen.queryByRole('alert')).toBeNull();

		// Once an error is present the hint gives way to it — never both at once.
		rerender({ label: 'Name', hint: 'As it appears on the gift', error: 'Required' });
		expect(screen.queryByText('As it appears on the gift')).toBeNull();
		expect(screen.getByRole('alert')).toHaveTextContent('Required');
	});
});
