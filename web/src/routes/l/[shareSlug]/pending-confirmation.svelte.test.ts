import { describe, it, expect, vi } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/svelte';
import Page, { pendingConfirmationOf } from './+page.svelte';
import type { ActionData, PageData } from './$types';

// #430. On an email_confirmed list a reserve is held pending the giver's confirmation.
// The hold is correct and unchanged — ADR-0007 §3 requires it to read as taken so no
// one races for the last unit — but the page told the giver they were not finished in
// one place only: a banner above the page header. The reserve button sits on an item
// row, which on a phone is well below the fold, so the instruction rendered above the
// giver's scroll position and nothing moved the viewport to it. The item's chip, which
// they could see, said Reserved.
//
// ⚠️ WHAT THIS SUITE CANNOT DO, stated because the gap is the whole point of the bug:
// jsdom performs no layout, so no assertion here can show that anything is above or
// below a fold. A suite that checked only "the instruction is in the document" would
// have passed against the broken page — the banner WAS in the document.
//
// So the property asserted instead is CONTAINMENT: the instruction renders inside the
// <li> of the item that was acted on. That is what makes being in view structural
// rather than measured — the giver's viewport is on that row because they just pressed
// a button in it, so no scrolling has to be asked for, detected, or measured. Moving
// the instruction back to the top of the document makes closest('li') null and fails
// these tests, which is the mutation that matters.

const INSTRUCTION = 'Almost there — check your email to confirm your reservation.';

const listData = (): PageData =>
	({
		closed: false as const,
		list: {
			title: 'Housewarming',
			event_date: null,
			email_required: true,
			items: [
				{ id: 'item-one', name: 'Cast iron pan', availability: 'reserved' },
				{ id: 'item-two', name: 'Striped blanket', availability: 'available' }
			]
		},
		reservedItemIds: [] as string[],
		pledged: {},
		noteHtml: {}
	}) as unknown as PageData;

// The shape the reserve action returns on the pending path. `message` is what the
// banner and the in-row panel both read, so a test that stops seeing it stops seeing
// the instruction in either place.
const pendingForm = (itemId: string, deadline: string | null) =>
	({
		form: { message: INSTRUCTION, data: { item_id: itemId }, errors: {}, valid: true },
		pendingConfirmation: { itemId, deadline }
	}) as unknown as ActionData;

// Substring matching, deliberately. The banner renders the message with a "✓ " in
// front of it and the in-row panel renders it bare, so an exact matcher sees the panel
// and is BLIND to the banner — which would make the duplicate-instruction assertion
// below silently unable to fail. Matching on the containing text sees both surfaces.
const instructionEls = () => screen.queryAllByText(INSTRUCTION, { exact: false });

const rowOf = (container: HTMLElement, name: string): HTMLElement => {
	const row = [...container.querySelectorAll('li')].find((li) => li.textContent?.includes(name));
	if (!row) throw new Error(`no row for ${name}`);
	return row as HTMLElement;
};

describe('the pending instruction is drawn in the row that was acted on (#430)', () => {
	it('puts the instruction inside the acted-on item row, not at the top of the page', () => {
		const { container } = render(Page, {
			data: listData(),
			form: pendingForm('item-one', '2026-09-22 20:28 CEST')
		});

		expect(instructionEls()).toHaveLength(1);
		const instruction = instructionEls()[0];
		const row = instruction.closest('li');
		// Both halves matter. Being in SOME row is not the claim — it has to be the row
		// of the item the giver reserved, which is what anchors it to their viewport.
		expect(row, 'the instruction must live inside an item row').not.toBeNull();
		expect(row).toHaveTextContent('Cast iron pan');
		expect(rowOf(container, 'Cast iron pan')).toContainElement(instruction);
	});

	it('says it exactly once, so the banner stands down when the row carries it', () => {
		// The failure this guards is a fix that ADDS the in-row copy and leaves the
		// banner in place: the placement bug would be invisible in a screenshot and the
		// giver would be told twice.
		render(Page, { data: listData(), form: pendingForm('item-one', null) });
		expect(instructionEls()).toHaveLength(1);
	});

	it('still tells the giver through the banner if the acted-on row is not on the page', () => {
		// An item that has gone from the list between the POST and this render has no
		// row to draw into. Suppressing the banner on the mere presence of a pending
		// result would then say nothing at all, which is worse than the bug.
		const { container } = render(Page, {
			data: listData(),
			form: pendingForm('item-gone', '2026-09-22 20:28 CEST')
		});
		expect(instructionEls()).toHaveLength(1);
		expect(instructionEls()[0].closest('li')).toBeNull();
		expect(container.querySelectorAll('li')).toHaveLength(2);
	});

	it('draws nothing on a render that is not a pending reserve', () => {
		// The control for every assertion above: without a pending result the row is
		// untouched, so none of this can be passing for an unrelated reason.
		render(Page, { data: listData(), form: null });
		expect(instructionEls()).toHaveLength(0);
		expect(screen.queryByText('Awaiting your confirmation')).toBeNull();
	});
});

describe('a pending reservation stops reading as a finished one (#430)', () => {
	it('relabels the acted-on row and leaves every other row alone', () => {
		const { container } = render(Page, {
			data: listData(),
			form: pendingForm('item-one', null)
		});

		const acted = rowOf(container, 'Cast iron pan');
		const other = rowOf(container, 'Striped blanket');

		// The word the giver reads as "done" must be gone from THEIR row...
		expect(acted).toHaveTextContent('Awaiting your confirmation');
		expect(acted).not.toHaveTextContent('Reserved');
		// ...and still be the signal everyone else's rows use. One render, each row the
		// other's control, so this cannot pass by relabelling the whole page.
		expect(other).not.toHaveTextContent('Awaiting your confirmation');
	});

	it('does not claim the reservation is yours to release', () => {
		// The pending path issues no capability token and stores no cookie, so the
		// browser cannot release this reservation. A row that offered it would be
		// offering an action that fails.
		const { container } = render(Page, { data: listData(), form: pendingForm('item-one', null) });
		const acted = rowOf(container, 'Cast iron pan');
		expect(acted).not.toHaveTextContent('You reserved this');
		expect(acted.querySelector('button[formaction="?/release"]')).toBeNull();
	});
});

describe('the deadline is named only when one exists (#430)', () => {
	it('shows exactly the string the server rendered, zone and all', () => {
		// The page formats nothing itself (#438): the confirm email states this same
		// deadline, a giver may hold both, and the zone is named so a reader
		// elsewhere does not take it for their own clock.
		render(Page, { data: listData(), form: pendingForm('item-one', '2026-09-22 20:28 CEST') });
		expect(
			screen.getByText(/Confirm by 2026-09-22 20:28 CEST, or the item is released/)
		).toBeInTheDocument();
	});

	it('passes an offset-named zone through unchanged too', () => {
		// Not every zone has a letter abbreviation; some render as an offset. The
		// page must not care — it prints what it was given.
		render(Page, { data: listData(), form: pendingForm('item-one', '2026-09-22 21:58 +0330') });
		expect(
			screen.getByText(/Confirm by 2026-09-22 21:58 \+0330, or the item is released/)
		).toBeInTheDocument();
	});

	it('says nothing about a deadline when the list has none', () => {
		// A zero effective confirm window disables the sweep, so the reservation waits
		// indefinitely and the backend sends no deadline. Naming one here would tell the
		// giver to act by a time at which nothing happens. The instruction itself stays.
		render(Page, { data: listData(), form: pendingForm('item-one', null) });
		expect(screen.queryByText(/Confirm by/)).toBeNull();
		expect(instructionEls()).toHaveLength(1);
	});
});

// The deadline string itself is no longer built here. It is rendered server-side
// in the instance's timezone (#438) and the page shows exactly what it was given,
// because the confirm email states the same deadline and a giver may hold both.
//
// That also retires a hazard rather than moving it: the old client formatter had
// to be tested with the timezone PINNED, since CI runs in UTC where a local-time
// implementation produces an identical string and the assertion could not fail.
// The Go formatter takes its location as an argument, so there is no ambient value
// for a test to inherit in the first place.

describe('pendingConfirmationOf', () => {
	it('reads the item and deadline the action sent', () => {
		expect(
			pendingConfirmationOf({
				pendingConfirmation: { itemId: 'i1', deadline: '2026-01-02T15:04:00Z' }
			})
		).toEqual({ itemId: 'i1', deadline: '2026-01-02T15:04:00Z' });
	});

	it('is undefined for every result that is not a pending reserve', () => {
		expect(pendingConfirmationOf(null)).toBeUndefined();
		expect(pendingConfirmationOf(undefined)).toBeUndefined();
		expect(pendingConfirmationOf({ form: { message: 'Reserved — thank you!' } })).toBeUndefined();
		expect(pendingConfirmationOf({ pledgeMessage: 'thanks' })).toBeUndefined();
	});

	it('refuses a payload with no usable item, since there would be no row to draw in', () => {
		expect(pendingConfirmationOf({ pendingConfirmation: null })).toBeUndefined();
		expect(pendingConfirmationOf({ pendingConfirmation: {} })).toBeUndefined();
		expect(pendingConfirmationOf({ pendingConfirmation: { itemId: '' } })).toBeUndefined();
		expect(pendingConfirmationOf({ pendingConfirmation: { itemId: 7 } })).toBeUndefined();
	});

	it('normalises a missing or non-string deadline to null rather than dropping the item', () => {
		// The item is what places the instruction; the deadline is an extra. Losing the
		// deadline must not lose the instruction with it.
		expect(pendingConfirmationOf({ pendingConfirmation: { itemId: 'i1' } })).toEqual({
			itemId: 'i1',
			deadline: null
		});
		expect(pendingConfirmationOf({ pendingConfirmation: { itemId: 'i1', deadline: 5 } })).toEqual({
			itemId: 'i1',
			deadline: null
		});
	});
});

// The action half: the page can only draw what the action hands it, and before this
// change it handed back a message and nothing else.
const state = vi.hoisted(() => ({ next: null as unknown }));
vi.mock('$lib/server/api', () => ({
	backendClient: () => ({ POST: async () => state.next })
}));

const { actions } = await import('./+page.server');

type ActionFn = (e: unknown) => Promise<unknown>;
const callReserve = (next: unknown) => {
	state.next = next;
	const fd = new FormData();
	fd.set('item_id', 'item-one');
	fd.set('giver_email', 'giver@example.invalid');
	return (actions.reserve as unknown as ActionFn)({
		request: new Request('http://t.example/l/s1?/reserve', { method: 'POST', body: fd }),
		params: { shareSlug: 's1' },
		locals: { host: 't.example' },
		cookies: { get: () => undefined, set: () => {}, delete: () => {} },
		url: new URL('http://t.example/l/s1')
	});
};

const pendingOf = (res: unknown) =>
	(res as { pendingConfirmation?: { itemId: string; deadline: string | null } })
		.pendingConfirmation;

describe('the reserve action hands the page what it needs to place the instruction (#430)', () => {
	it('returns the acted-on item and the deadline alongside the message', async () => {
		const res = await callReserve({
			data: {
				reservation_id: 'r1',
				status: 'pending_confirmation',
				confirm_deadline: '2026-09-22T18:28:00Z',
				confirm_deadline_display: '2026-09-22 20:28 CEST'
			},
			error: undefined,
			response: { status: 202 }
		});
		// The DISPLAY string is what travels, not the instant. Passing the raw
		// instant through would make the page format it a second time.
		expect(pendingOf(res)).toEqual({
			itemId: 'item-one',
			deadline: '2026-09-22 20:28 CEST'
		});
		// The message is unchanged — this adds a key, it does not move the text.
		expect((res as { form?: { message?: string } }).form?.message).toBe(INSTRUCTION);
	});

	it('carries a null deadline when the backend states none', async () => {
		const res = await callReserve({
			data: { reservation_id: 'r1', status: 'pending_confirmation' },
			error: undefined,
			response: { status: 202 }
		});
		expect(pendingOf(res)).toEqual({ itemId: 'item-one', deadline: null });
	});

	it('states no deadline when the backend sends an instant but no rendering of it', async () => {
		// A backend older than the timezone setting sends confirm_deadline and no
		// display string. Showing nothing is the right degradation: the page cannot
		// render the instant without re-introducing a second formatter, and the
		// instruction itself is unaffected.
		const res = await callReserve({
			data: {
				reservation_id: 'r1',
				status: 'pending_confirmation',
				confirm_deadline: '2026-09-22T18:28:00Z'
			},
			error: undefined,
			response: { status: 202 }
		});
		expect(pendingOf(res)).toEqual({ itemId: 'item-one', deadline: null });
	});

	it('sends no pending payload on a reservation that is already active', async () => {
		// The full_guest path is finished the moment it returns, so nothing here should
		// change for it — the control that keeps this scoped to the email-confirm tier.
		const res = await callReserve({
			data: { reservation_id: 'r1', status: 'active', capability_token: 'tok' },
			error: undefined,
			response: { status: 201 }
		});
		expect(pendingOf(res)).toBeUndefined();
		expect((res as { form?: { message?: string } }).form?.message).toBe('Reserved — thank you!');
	});
});
