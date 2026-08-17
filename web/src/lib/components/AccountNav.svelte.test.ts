import { describe, it, expect } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/svelte';
import AccountNav from './AccountNav.svelte';

describe('AccountNav', () => {
	it('renders the identity and the action group, gating Admin on is_admin', () => {
		const { rerender } = render(AccountNav, { name: 'Ada', isAdmin: false, pathname: '/x' });
		expect(screen.getByText('Ada')).toBeInTheDocument();
		expect(screen.getByRole('link', { name: 'Reserved' })).toBeInTheDocument();
		expect(screen.getByRole('link', { name: 'Settings' })).toBeInTheDocument();
		expect(screen.queryByRole('link', { name: 'Admin' })).toBeNull();
		expect(screen.getByRole('button', { name: 'Log out' })).toBeInTheDocument();

		rerender({ name: 'Ada', isAdmin: true, pathname: '/x' });
		expect(screen.getByRole('link', { name: 'Admin' })).toBeInTheDocument();
	});

	it('marks the current route active with aria-current and only that one', () => {
		// Read the real hrefs (whatever resolve() produces) and drive the active
		// state off them, so the test is about the matching logic, not the paths.
		const { rerender } = render(AccountNav, { name: 'Ada', isAdmin: false, pathname: '/nowhere' });
		const reserved = screen.getByRole('link', { name: 'Reserved' });
		const settings = screen.getByRole('link', { name: 'Settings' });
		expect(reserved).not.toHaveAttribute('aria-current');
		expect(settings).not.toHaveAttribute('aria-current');

		const reservedHref = reserved.getAttribute('href') as string;
		rerender({ name: 'Ada', isAdmin: false, pathname: reservedHref });
		expect(screen.getByRole('link', { name: 'Reserved' })).toHaveAttribute('aria-current', 'page');
		expect(screen.getByRole('link', { name: 'Settings' })).not.toHaveAttribute('aria-current');
	});

	it('matches a nested sub-path of the active section', () => {
		const { rerender } = render(AccountNav, { name: 'Ada', isAdmin: false, pathname: '/nowhere' });
		const settingsHref = screen
			.getByRole('link', { name: 'Settings' })
			.getAttribute('href') as string;
		rerender({ name: 'Ada', isAdmin: false, pathname: `${settingsHref}/security` });
		expect(screen.getByRole('link', { name: 'Settings' })).toHaveAttribute('aria-current', 'page');
	});

	it('renders a divider between the identity and the actions', () => {
		const { container } = render(AccountNav, { name: 'Ada', isAdmin: false, pathname: '/x' });
		expect(container.querySelector('[aria-hidden="true"]')).not.toBeNull();
	});
});
