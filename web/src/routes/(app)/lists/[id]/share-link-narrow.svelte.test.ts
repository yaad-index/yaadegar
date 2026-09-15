import { describe, it, expect, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/svelte';
import { page } from '$app/state';
import Page from './+page.svelte';
import type { PageData } from './$types';

// #393: the share-link input is a flex child with flex-1, and a flex child defaults
// to min-width:auto. For a bare <input> that floors at the intrinsic width of its
// `size` — 20 characters by default, 244px at this input's type size — so it refuses
// to shrink and pushes the Copy button past the right edge of the page.
//
// ⚠️ What this test is, and is not. jsdom performs no layout: every width here is
// zero, so the overflow itself cannot be asserted in this suite at all. The proof is
// a browser measurement, recorded in the PR — document.scrollWidth 367 against
// viewports of 360 and 320 with the page at rest, 360/320 after the fix, and a
// control showing min-width:0 on the ROW changes nothing, which is what localises
// the floor to the input.
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
		items: [],
		registrationEnabled: false,
		noteHtml: {},
		descriptionHtml: '',
		addForm: { id: 'add', valid: true, posted: false, errors: {}, data: {} }
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
	}) as any;

describe('share-link row on a narrow viewport (#393)', () => {
	beforeEach(() => {
		// The share panel lives on the items tab, which is the default; set the URL
		// explicitly so this does not depend on whatever tab a previous test left.
		page.url = new URL('http://test.local/lists/l1') as typeof page.url;
	});

	it('gives the flex-1 share input min-w-0 so it can shrink below its intrinsic width', () => {
		render(Page, { props: { data: listData(), form: null } });
		const input = screen.getByLabelText('Public share link');
		expect(input).toHaveClass('flex-1');
		expect(input).toHaveClass('min-w-0');
	});

	it('keeps Copy in the same flex row as the input', () => {
		// The row is a shrinkable box beside an intrinsically-sized button. If Copy
		// ever moved out of this row, the fix above would be addressing a shape that
		// no longer exists.
		render(Page, { props: { data: listData(), form: null } });
		const input = screen.getByLabelText('Public share link');
		const copy = screen.getByRole('button', { name: 'Copy' });
		expect(input.parentElement).toHaveClass('flex');
		expect(copy.parentElement).toBe(input.parentElement);
	});
});
