import { describe, it, expect, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import { page } from '$app/state';
import Page from './+page.svelte';
import type { PageData } from './$types';

// #390: the edit-item name input is a flex child with flex-1, and a flex child
// defaults to min-width:auto. For a bare <input> that floors at the intrinsic width
// of its `size` — 20 characters by default, 274px in this typeface — so the input
// refuses to shrink and pushes the 80px quantity box off the card on a narrow
// viewport.
//
// ⚠️ What this test is, and is not. jsdom performs no layout: every width here is
// zero, so the overflow itself cannot be asserted in this suite at all. The proof is
// a browser measurement, recorded in the PR — baseline 412px against a 390px
// viewport, 390px after the fix, and a control showing min-width:0 on the ROW
// changes nothing, which is what localises the floor to the input.
//
// What these assertions buy is a regression pin: the class is load-bearing and looks
// like tidiness, so the failure mode worth guarding is someone removing it during an
// unrelated cleanup. That is a real risk and this catches it; it is not evidence the
// layout is correct.

const listData = (): PageData =>
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
				name: 'Ceramic pour-over coffee dripper',
				quantity_wanted: 2,
				reserved_quantity: 0,
				availability: 'available',
				priority: 0
			}
		],
		registrationEnabled: false,
		noteHtml: {},
		descriptionHtml: '',
		addForm: { id: 'add', valid: true, posted: false, errors: {}, data: {} }
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
	}) as any;

// Queries are scoped to the edit form: the add-item form above it also has a field
// labelled "Item name", and a page-wide lookup would silently assert against the row
// that was never broken.
async function openEditForm() {
	const { container } = render(Page, { props: { data: listData(), form: null } });
	const edit = screen.getAllByRole('button', { name: 'Edit' })[0];
	await fireEvent.click(edit);
	const form = container.querySelector('form[action="?/edit"]');
	if (!form) throw new Error('the edit form did not open');
	return form;
}

describe('edit-item row on a narrow viewport (#390)', () => {
	beforeEach(() => {
		// The items tab is the default; set the URL explicitly so this does not depend
		// on whatever tab a previous test left behind.
		page.url = new URL('http://test.local/lists/l1') as typeof page.url;
	});

	it('gives the flex-1 name input min-w-0 so it can shrink below its intrinsic width', async () => {
		const form = await openEditForm();
		const name = form.querySelector('input[aria-label="Item name"]');
		expect(name).toHaveClass('flex-1');
		expect(name).toHaveClass('min-w-0');
	});

	it('keeps the quantity input at its fixed width beside it', async () => {
		// The row is a fixed-width box next to a shrinkable one. If the quantity input
		// ever became flexible instead, the fix above would be addressing a row shape
		// that no longer exists.
		const form = await openEditForm();
		const qty = form.querySelector('input[aria-label="Quantity"]');
		expect(qty).toHaveClass('w-20');
		expect(qty?.parentElement).toHaveClass('flex');
	});
});
