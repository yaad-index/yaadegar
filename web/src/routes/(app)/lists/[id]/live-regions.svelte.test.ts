import { describe, it, expect, beforeEach, vi } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import { page } from '$app/state';
import Page from './+page.svelte';
import type { PageData, ActionData } from './$types';

// #428: every role="status" region on this page used to sit inside an {#if}, so it
// entered the accessibility tree already holding its message. Assistive technology
// announces a CHANGE to a live region it already knows about; one inserted with its
// text is announced unreliably or not at all.
//
// A component test cannot hear an announcement. What it can pin is the precondition
// for one: the region is in the document before the result, and the result lands in
// that SAME element. Each test below holds the element from the empty render and
// checks it afterwards, so a region that is re-created (the old {#if} shape) fails
// even though the text ends up on the page either way.

const listData = (registrationEnabled = true): PageData =>
	({
		list: {
			id: 'l1',
			title: 'Birthday',
			description: '',
			thank_you_template: '',
			allow_cobuy: true,
			reserver_tier: null,
			visibility: 'private',
			share_slug: 'abc123'
		},
		items: [
			{
				id: 'i1',
				name: 'Still wanted',
				quantity_wanted: 1,
				reserved_quantity: 0,
				availability: 'available',
				priority: 0
			}
		],
		registrationEnabled,
		noteHtml: {},
		descriptionHtml: '',
		addForm: { id: 'add', valid: true, posted: false, errors: {}, data: {} }
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
	}) as any;

const regions = () => screen.getAllByRole('status');

describe('live regions exist before they have anything to say (#428)', () => {
	describe('list tab', () => {
		beforeEach(() => {
			page.url = new URL('http://test.local/lists/l1') as typeof page.url;
		});

		it('renders its status regions empty before any action', () => {
			render(Page, { props: { data: listData(), form: null } });
			expect(regions()).toHaveLength(2);
			for (const region of regions()) expect(region.textContent?.trim()).toBe('');
		});

		it('writes the archive result into the region that was already there', async () => {
			const { rerender } = render(Page, { props: { data: listData(), form: null } });
			const before = regions();

			await rerender({
				data: listData(),
				form: { archived: true, archivedName: 'Still wanted', archiveWarnings: [] } as ActionData
			});

			const filled = before.filter((el) => el.textContent?.includes('Archived “Still wanted”'));
			expect(filled).toHaveLength(1);
			expect(filled[0].isConnected).toBe(true);
		});

		it('writes the put-back result into the region that was already there', async () => {
			const { rerender } = render(Page, { props: { data: listData(), form: null } });
			const before = regions();

			await rerender({
				data: listData(),
				form: { unarchived: true, archivedName: 'Still wanted' } as ActionData
			});

			const filled = before.filter((el) => el.textContent?.includes('back on your list'));
			expect(filled).toHaveLength(1);
			expect(filled[0].isConnected).toBe(true);
		});

		it('writes the copy-link outcome into the region that was already there', async () => {
			const writeText = vi.fn().mockResolvedValue(undefined);
			vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText } });
			try {
				render(Page, { props: { data: listData(), form: null } });
				const before = regions();

				await fireEvent.click(screen.getByRole('button', { name: 'Copy' }));
				await vi.waitFor(() => expect(writeText).toHaveBeenCalled());

				await vi.waitFor(() =>
					expect(before.filter((el) => el.textContent?.includes('Link copied.'))).toHaveLength(1)
				);
			} finally {
				vi.unstubAllGlobals();
			}
		});

		// An error is announced assertively, as the settings and import errors on the
		// Settings tab already are; a failed archive is not a polite status update.
		it('reports a failed archive as an alert', () => {
			render(Page, {
				props: {
					data: listData(),
					form: { archiveError: 'Could not archive that item.' } as ActionData
				}
			});
			expect(screen.getByRole('alert')).toHaveTextContent('Could not archive that item.');
			for (const region of regions()) expect(region.textContent?.trim()).toBe('');
		});
	});

	describe('settings tab', () => {
		beforeEach(() => {
			page.url = new URL('http://test.local/lists/l1?tab=settings') as typeof page.url;
		});

		it('renders its status regions empty before any action', () => {
			render(Page, { props: { data: listData(false), form: null } });
			expect(regions()).toHaveLength(3);
			for (const region of regions()) expect(region.textContent?.trim()).toBe('');
		});

		it('writes "Settings saved." into the region that was already there', async () => {
			const { rerender } = render(Page, { props: { data: listData(), form: null } });
			const before = regions();

			await rerender({ data: listData(), form: { settingsSaved: true } as ActionData });

			expect(before.filter((el) => el.textContent?.includes('Settings saved.'))).toHaveLength(1);
		});

		it('writes the import count into the region that was already there', async () => {
			const { rerender } = render(Page, { props: { data: listData(), form: null } });
			const before = regions();

			await rerender({ data: listData(), form: { imported: 3 } as ActionData });

			expect(before.filter((el) => el.textContent?.includes('Imported 3 item(s).'))).toHaveLength(
				1
			);
		});

		it('writes the registration warning into the region that was already there', async () => {
			const { container } = render(Page, { props: { data: listData(false), form: null } });
			const before = regions();

			await fireEvent.change(container.querySelector('#list-reserver-tier')!, {
				target: { value: 'registered' }
			});

			expect(
				before.filter((el) => el.textContent?.includes('Self-registration is disabled'))
			).toHaveLength(1);
		});
	});
});
