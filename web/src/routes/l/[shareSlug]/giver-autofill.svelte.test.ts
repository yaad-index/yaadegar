import { describe, it, expect } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render } from '@testing-library/svelte';
import Page from './+page.svelte';
import type { PageData } from './$types';

// #434. On mobile the giver email field offered no saved address, although type and
// autocomplete were already correct — so the obvious cause was ruled out before this
// change and adding those attributes was never the fix.
//
// ⚠️ WHAT THIS SUITE CANNOT DO. Autofill is browser behaviour driven by per-engine
// heuristics and the viewer's own saved profile. jsdom has neither, so NOTHING here
// can show that a saved address is offered — that is the issue's acceptance criterion
// and it is a real-device check, not this file's. Asserting "autofill works" here
// would be an assertion that cannot fail.
//
// What this suite CAN do is hold the page at the shape the heuristics are given, and
// in particular pin the two structural facts that were measured while diagnosing it,
// because both are things a later refactor could quietly undo:
//   1. ONE giver_email input in the document however many items the list has. The form
//      wraps the item loop; moving it inside would put one identity block in every row
//      and give every engine a pile of identical candidates to choose between.
//   2. The identity fields render on first paint, not behind an interaction — several
//      engines evaluate candidacy once and do not re-evaluate.

const items = (n: number) =>
	Array.from({ length: n }, (_, i) => ({
		id: `item-${i}`,
		name: `Gift ${i}`,
		availability: 'available'
	}));

const listData = (itemCount: number): PageData =>
	({
		closed: false as const,
		list: {
			title: 'Housewarming',
			event_date: null,
			email_required: true,
			items: items(itemCount)
		},
		reservedItemIds: [] as string[],
		pledged: {},
		noteHtml: {},
		pendingConfirmations: []
	}) as unknown as PageData;

const emailInput = (c: HTMLElement) =>
	c.querySelector('input[name="giver_email"]') as HTMLInputElement | null;
const nameInput = (c: HTMLElement) =>
	c.querySelector('input[name="giver_name"]') as HTMLInputElement | null;

describe('the giver identity fields are shaped for autofill (#434)', () => {
	it('renders exactly one giver_email input however long the list is', () => {
		for (const count of [1, 3, 12]) {
			const { container, unmount } = render(Page, { data: listData(count), form: null });
			expect(container.querySelectorAll('input[name="giver_email"]')).toHaveLength(1);
			expect(container.querySelectorAll('input[name="giver_name"]')).toHaveLength(1);
			unmount();
		}
	});

	it('carries the id, name, type and autocomplete an engine keys on', () => {
		const { container } = render(Page, { data: listData(3), form: null });
		const email = emailInput(container);
		expect(email).toHaveAttribute('id', 'giver-email');
		expect(email).toHaveAttribute('type', 'email');
		expect(email).toHaveAttribute('autocomplete', 'email');
		expect(email).toHaveAttribute('inputmode', 'email');

		const name = nameInput(container);
		expect(name).toHaveAttribute('id', 'giver-name');
		expect(name).toHaveAttribute('type', 'text');
		expect(name).toHaveAttribute('autocomplete', 'name');
	});

	it('associates each label with its field explicitly', () => {
		const { container } = render(Page, { data: listData(3), form: null });
		for (const input of [emailInput(container), nameInput(container)]) {
			expect(input?.labels).toHaveLength(1);
			expect(input?.labels?.[0].getAttribute('for')).toBe(input?.getAttribute('id'));
		}
	});

	it('gives every id in the document a single owner', () => {
		const { container } = render(Page, { data: listData(12), form: null });
		const ids = [...container.querySelectorAll('[id]')].map((el) => el.id);
		expect(ids).toHaveLength(new Set(ids).size);
	});

	it('never suppresses autofill at the form level', () => {
		const { container } = render(Page, { data: listData(3), form: null });
		const form = container.querySelector('form');
		// autocomplete="off" on the form is the one attribute that overrides every
		// correct token on the fields inside it.
		expect(form?.getAttribute('autocomplete')).not.toBe('off');
		expect(emailInput(container)?.getAttribute('autocomplete')).not.toBe('off');
	});

	it('renders the fields on first paint, with no interaction', () => {
		const { container } = render(Page, { data: listData(3), form: null });
		// Queried immediately after render: nothing has been clicked, opened, or waited
		// for. An engine that evaluates candidacy once still sees these.
		expect(emailInput(container)).toBeInTheDocument();
		expect(emailInput(container)).toBeVisible();
	});
});
