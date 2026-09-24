import { describe, it, expect } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/svelte';
import Page, { PENDING_INSTRUCTION } from './+page.svelte';
import type { ActionData, PageData } from './$types';

// #441. #430 put the instruction in the row that was acted on, but everything it had
// to work with came from the action result, which exists for exactly one render. A
// giver who reloads, navigates back, or presses "Check for updates" before confirming
// was shown a plain `Reserved` chip and no instruction at all, while the hold was
// still pending and its confirm window still running. The page had forgotten what it
// had told them.
//
// The state now also arrives on `data`, read server-side from a marker stored for the
// browser that made the reservation. So every test here passes NO form at all: that is
// the whole point — the reload path is the one with no action result in it.

const listData = (
	pendingConfirmations: { itemId: string; deadlineDisplay: string | null }[]
): PageData =>
	({
		closed: false as const,
		list: {
			title: 'Housewarming',
			event_date: null,
			email_required: true,
			items: [
				{ id: 'item-one', name: 'Cast iron pan', availability: 'reserved' },
				// Deliberately `available` while carrying a pending marker below. A pending
				// hold counts toward reserved quantity, but deriveAvailability only reports
				// `reserved` once claimed units MEET the wanted quantity — so a multi-unit
				// item with one pending hold looks `available` to everyone, including the
				// giver holding it.
				{ id: 'item-two', name: 'Striped blanket', availability: 'available' }
			]
		},
		reservedItemIds: [] as string[],
		pledged: {},
		noteHtml: {},
		pendingConfirmations
	}) as unknown as PageData;

const instructionEls = () => screen.queryAllByText(PENDING_INSTRUCTION, { exact: false });

const rowOf = (container: HTMLElement, name: string): HTMLElement => {
	const row = [...container.querySelectorAll('li')].find((li) => li.textContent?.includes(name));
	if (!row) throw new Error(`no row for ${name}`);
	return row as HTMLElement;
};

describe('a pending reservation survives a reload (#441)', () => {
	it('still tells the giver to confirm when no action ran', () => {
		const { container } = render(Page, {
			data: listData([{ itemId: 'item-one', deadlineDisplay: '2026-09-22 20:28 CEST' }]),
			form: null
		});

		expect(instructionEls()).toHaveLength(1);
		// Same containment property #430 established: in the row, not at the top of the
		// document. A reload does not move the viewport either.
		expect(instructionEls()[0].closest('li')).toBe(rowOf(container, 'Cast iron pan'));
	});

	it('relabels that row’s chip instead of leaving it reading Reserved', () => {
		const { container } = render(Page, {
			data: listData([{ itemId: 'item-one', deadlineDisplay: '2026-09-22 20:28 CEST' }]),
			form: null
		});
		const row = rowOf(container, 'Cast iron pan');
		expect(row).toHaveTextContent('Awaiting your confirmation');
		// The defect in #430 was that `Reserved` is the strongest thing on the row and
		// reads as "done". It must not still be there next to the new chip.
		expect(row.textContent).not.toMatch(/\bReserved\b/);
	});

	it('shows exactly the deadline string the server rendered, zone and all', () => {
		// #438: the page formats nothing. The instance renders the instant once, in its
		// own timezone and naming that zone, and the confirm email states the same
		// words — a giver may be holding both.

		render(Page, {
			data: listData([{ itemId: 'item-one', deadlineDisplay: '2026-09-22 20:28 CEST' }]),
			form: null
		});
		expect(screen.getByText(/Confirm by 2026-09-22 20:28 CEST/)).toBeInTheDocument();
	});

	it('names no deadline when the list has no confirm window', () => {
		render(Page, { data: listData([{ itemId: 'item-one', deadlineDisplay: null }]), form: null });
		// The instruction still stands — the hold waits indefinitely — but naming a time
		// would tell the giver to act by an instant at which nothing happens.
		expect(instructionEls()).toHaveLength(1);
		expect(screen.queryByText(/Confirm by/)).not.toBeInTheDocument();
	});

	// The regression guard for a guard deliberately NOT added. Filtering the marker on
	// `availability === 'reserved'` looks tidier and passes every other test in this
	// file; it silently hides the instruction on exactly the items where a pending hold
	// does not saturate the quantity.
	it('shows the instruction even while the item still reads as available', () => {
		const { container } = render(Page, {
			data: listData([{ itemId: 'item-two', deadlineDisplay: '2026-09-22 20:28 CEST' }]),
			form: null
		});
		expect(instructionEls()).toHaveLength(1);
		expect(instructionEls()[0].closest('li')).toBe(rowOf(container, 'Striped blanket'));
	});

	it('marks each pending row when more than one is waiting', () => {
		const { container } = render(Page, {
			data: listData([
				{ itemId: 'item-one', deadlineDisplay: '2026-09-22 20:28 CEST' },
				{ itemId: 'item-two', deadlineDisplay: null }
			]),
			form: null
		});
		expect(instructionEls()).toHaveLength(2);
		expect(rowOf(container, 'Cast iron pan')).toHaveTextContent('Awaiting your confirmation');
		expect(rowOf(container, 'Striped blanket')).toHaveTextContent('Awaiting your confirmation');
	});

	it('says nothing at all when this browser has no pending hold', () => {
		const { container } = render(Page, { data: listData([]), form: null });
		expect(instructionEls()).toHaveLength(0);
		expect(rowOf(container, 'Cast iron pan')).toHaveTextContent('Reserved');
	});

	it('ignores a marker for an item no longer on the list', () => {
		const { container } = render(Page, {
			data: listData([{ itemId: 'item-gone', deadlineDisplay: '2026-09-22 20:28 CEST' }]),
			form: null
		});
		expect(instructionEls()).toHaveLength(0);
		expect(container.querySelectorAll('li').length).toBeGreaterThan(0);
	});

	it('does not offer a release for a hold that has no capability yet', () => {
		const { container } = render(Page, {
			data: listData([{ itemId: 'item-one', deadlineDisplay: '2026-09-22 20:28 CEST' }]),
			form: null
		});
		// The marker carries no token by construction, so the page must not present an
		// action that would need one. reservedItemIds is what drives Release, and a
		// pending hold is correctly absent from it.
		//
		// Asserted on the row's CONTROLS, not its text: the deadline sentence contains
		// the word "released", so a text match here passes or fails on prose rather than
		// on whether a release is actually offered.
		const controls = [
			...rowOf(container, 'Cast iron pan').querySelectorAll('button, input[type="submit"]')
		];
		expect(controls.map((c) => c.textContent?.trim() ?? '')).not.toContain('Release');
		expect(
			controls.filter((c) => /release/i.test(c.getAttribute('formaction') ?? ''))
		).toHaveLength(0);
	});
});

describe('the reload path and the just-reserved path say the same thing (#441)', () => {
	const deadline = '2026-09-22 20:28 CEST';
	const actionResult = (itemId: string) =>
		({
			form: { message: PENDING_INSTRUCTION, data: { item_id: itemId }, errors: {}, valid: true },
			pendingConfirmation: { itemId, deadline }
		}) as unknown as ActionData;

	it('renders one identical instruction whether or not an action just ran', () => {
		const reload = render(Page, {
			data: listData([{ itemId: 'item-one', deadlineDisplay: deadline }]),
			form: null
		});
		const reloadText = instructionEls().map((el) => el.textContent?.trim());
		reload.unmount();

		render(Page, { data: listData([]), form: actionResult('item-one') });
		const actionText = instructionEls().map((el) => el.textContent?.trim());

		expect(reloadText).toEqual(actionText);
		expect(reloadText).toHaveLength(1);
	});

	it('does not say it twice when the marker and the action agree', () => {
		// The reserve that just happened is also stored, so on the render straight after
		// the POST both sources name the same item. Two sources must not become two
		// instructions.
		render(Page, {
			data: listData([{ itemId: 'item-one', deadlineDisplay: deadline }]),
			form: actionResult('item-one')
		});
		expect(instructionEls()).toHaveLength(1);
	});
});
