import { describe, it, expect } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import PasswordField from './PasswordField.svelte';

// The visibility toggle is a reactive interaction — its whole point is that a click
// flips the real input type — so it is exercised here in the DOM, not read from the
// diff (the #201 runtime-verification requirement, at the component level).
describe('PasswordField', () => {
	it('starts masked and toggles the input between password and text', async () => {
		render(PasswordField, { label: 'Password', name: 'password' });
		const input = screen.getByLabelText('Password');
		expect(input).toHaveAttribute('type', 'password');

		const toggle = screen.getByRole('button', { name: 'Show password' });
		expect(toggle).toHaveAttribute('aria-pressed', 'false');

		await fireEvent.click(toggle);
		expect(input).toHaveAttribute('type', 'text');
		expect(screen.getByRole('button', { name: 'Hide password' })).toHaveAttribute(
			'aria-pressed',
			'true'
		);

		await fireEvent.click(screen.getByRole('button', { name: 'Hide password' }));
		expect(input).toHaveAttribute('type', 'password');
	});

	it('passes native attributes through without changing the form contract', () => {
		render(PasswordField, {
			label: 'Password',
			name: 'password',
			autocomplete: 'new-password',
			required: true
		});
		const input = screen.getByLabelText('Password');
		expect(input).toHaveAttribute('name', 'password');
		expect(input).toHaveAttribute('autocomplete', 'new-password');
		expect(input).toBeRequired();
	});

	it('announces an error, marks the input invalid, and shows a visible focus ring', () => {
		render(PasswordField, { label: 'Password', name: 'password', error: 'Too short' });
		const alert = screen.getByRole('alert');
		expect(alert).toHaveTextContent('Too short');
		const input = screen.getByLabelText('Password');
		expect(input).toHaveAttribute('aria-invalid', 'true');
		expect(input).toHaveAttribute('aria-describedby', alert.id);
		// Same WCAG 2.4.7 guard as Field: focus produces a ring, not an inert re-border.
		expect(input.className).toContain('focus-visible:ring-red-500');
	});
});
