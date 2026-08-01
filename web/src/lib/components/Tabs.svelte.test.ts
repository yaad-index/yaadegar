import { describe, it, expect } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import TabsHarness from './TabsHarness.svelte';

// Runtime (real DOM) tests for the tab bar. The #128 regression was that clicking a
// tab did not switch the visible panel — a logic-only review could not catch it, so
// these exercise the actual click → render path.
describe('Tabs', () => {
	it('switches the visible panel when a tab is clicked', async () => {
		render(TabsHarness);
		expect(screen.getByTestId('panel')).toHaveTextContent('list-panel');

		await fireEvent.click(screen.getByRole('link', { name: 'Settings' }));
		expect(screen.getByTestId('panel')).toHaveTextContent('settings-panel');

		await fireEvent.click(screen.getByRole('link', { name: 'List' }));
		expect(screen.getByTestId('panel')).toHaveTextContent('list-panel');
	});

	it('honors the initial active tab (deep-link / reload)', () => {
		render(TabsHarness, { initial: 'settings' });
		expect(screen.getByTestId('panel')).toHaveTextContent('settings-panel');
	});

	it('marks the active tab with aria-current', async () => {
		render(TabsHarness);
		expect(screen.getByRole('link', { name: 'List' })).toHaveAttribute('aria-current', 'page');
		expect(screen.getByRole('link', { name: 'Settings' })).not.toHaveAttribute('aria-current');

		await fireEvent.click(screen.getByRole('link', { name: 'Settings' }));
		expect(screen.getByRole('link', { name: 'Settings' })).toHaveAttribute('aria-current', 'page');
		expect(screen.getByRole('link', { name: 'List' })).not.toHaveAttribute('aria-current');
	});

	it('notifies onselect with the chosen tab id', async () => {
		render(TabsHarness);
		await fireEvent.click(screen.getByRole('link', { name: 'Settings' }));
		expect(screen.getByTestId('selected')).toHaveTextContent('settings');
	});

	it('preserves in-progress input across a tab switch and back', async () => {
		render(TabsHarness);
		const draft = () => screen.getByLabelText('draft') as HTMLInputElement;
		await fireEvent.input(draft(), { target: { value: 'half-typed' } });

		await fireEvent.click(screen.getByRole('link', { name: 'Settings' }));
		await fireEvent.click(screen.getByRole('link', { name: 'List' }));

		expect(draft().value).toBe('half-typed');
	});
});
