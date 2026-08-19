import { describe, it, expect } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import MobileNav from './MobileNav.svelte';

// The drawer content only exists while open, so most assertions open it first.
async function openDrawer() {
	await fireEvent.click(screen.getByRole('button', { name: 'Open menu' }));
}

describe('MobileNav', () => {
	it('is collapsed until the hamburger is pressed, then lists the destinations, gating Admin on is_admin', async () => {
		const { rerender } = render(MobileNav, { name: 'Ada', isAdmin: false, pathname: '/x' });
		// Collapsed: the trigger is present, the drawer is not.
		expect(screen.getByRole('button', { name: 'Open menu' })).toBeInTheDocument();
		expect(screen.queryByRole('dialog')).toBeNull();

		await openDrawer();
		expect(screen.getByRole('dialog')).toBeInTheDocument();
		expect(screen.getByText('Ada')).toBeInTheDocument();
		expect(screen.getByRole('link', { name: 'My lists' })).toBeInTheDocument();
		expect(screen.getByRole('link', { name: 'Reserved' })).toBeInTheDocument();
		expect(screen.getByRole('link', { name: 'Settings' })).toBeInTheDocument();
		expect(screen.queryByRole('link', { name: 'Admin' })).toBeNull();
		expect(screen.getByRole('button', { name: 'Log out' })).toBeInTheDocument();

		// The drawer stays open across a prop change; Admin appears for an admin.
		await rerender({ name: 'Ada', isAdmin: true, pathname: '/x' });
		expect(screen.getByRole('link', { name: 'Admin' })).toBeInTheDocument();
	});

	it('marks the current route active with aria-current, and Home only on the exact root', async () => {
		const { rerender } = render(MobileNav, { name: 'Ada', isAdmin: false, pathname: '/nowhere' });
		await openDrawer();
		const myLists = screen.getByRole('link', { name: 'My lists' });
		const reserved = screen.getByRole('link', { name: 'Reserved' });
		expect(myLists).not.toHaveAttribute('aria-current');
		expect(reserved).not.toHaveAttribute('aria-current');

		// Read the real hrefs so the test is about the matching logic, not the paths.
		const homeHref = myLists.getAttribute('href') as string;
		const reservedHref = reserved.getAttribute('href') as string;

		// On a section route: that section is active and Home is not (Home is exact-match,
		// so it must not light up just because every path startsWith('/')).
		await rerender({ name: 'Ada', isAdmin: false, pathname: reservedHref });
		expect(screen.getByRole('link', { name: 'Reserved' })).toHaveAttribute('aria-current', 'page');
		expect(screen.getByRole('link', { name: 'My lists' })).not.toHaveAttribute('aria-current');

		// A nested sub-path still matches its section.
		await rerender({ name: 'Ada', isAdmin: false, pathname: `${reservedHref}/42` });
		expect(screen.getByRole('link', { name: 'Reserved' })).toHaveAttribute('aria-current', 'page');

		// On the root, Home is active.
		await rerender({ name: 'Ada', isAdmin: false, pathname: homeHref });
		expect(screen.getByRole('link', { name: 'My lists' })).toHaveAttribute('aria-current', 'page');
	});

	it('closes on Escape', async () => {
		render(MobileNav, { name: 'Ada', isAdmin: false, pathname: '/x' });
		await openDrawer();
		expect(screen.getByRole('dialog')).toBeInTheDocument();
		await fireEvent.keyDown(window, { key: 'Escape' });
		expect(screen.queryByRole('dialog')).toBeNull();
	});
});
