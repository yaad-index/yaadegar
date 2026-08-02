import { describe, it, expect } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/svelte';
import Page, { reserveNeedsEmail } from './+page.svelte';
import type { PageData } from './$types';

// #144: on an email-confirm list the backend rejects a reservation with no giver
// email. reserveNeedsEmail is the client-side guard that blocks that submit up front;
// the page also marks the field required so the giver sees it before submitting.

describe('reserveNeedsEmail guard', () => {
	it('blocks a reserve with no email when the list requires one', () => {
		expect(reserveNeedsEmail('?/reserve', true, '')).toBe(true);
		expect(reserveNeedsEmail('?/reserve', true, '   ')).toBe(true);
	});

	it('allows a reserve once an email is present', () => {
		expect(reserveNeedsEmail('?/reserve', true, 'g@example.com')).toBe(false);
	});

	it('never blocks when the list does not require an email', () => {
		expect(reserveNeedsEmail('?/reserve', false, '')).toBe(false);
	});

	it('only gates reserve — release/withdraw/pledge submit freely', () => {
		expect(reserveNeedsEmail('?/release', true, '')).toBe(false);
		expect(reserveNeedsEmail('?/withdraw', true, '')).toBe(false);
		expect(reserveNeedsEmail('?/pledge', true, '')).toBe(false);
	});
});

// The giver page reads only list/reservedItemIds/pledged/noteHtml; the load-provided
// superforms (form/pledgeForm) are unused by this component, so a render fixture casts
// past them rather than constructing full SuperValidated objects.
const listData = (emailRequired: boolean): PageData =>
	({
		closed: false as const,
		list: { title: 'Birthday', event_date: null, email_required: emailRequired, items: [] },
		reservedItemIds: [] as string[],
		pledged: {},
		noteHtml: {}
	}) as unknown as PageData;

describe('giver page email-required labeling', () => {
	it('marks the email field required on an email-confirm list', () => {
		const { container } = render(Page, { data: listData(true), form: null });
		expect(screen.getByText('Email (required)')).toBeInTheDocument();
		expect(screen.getByText('Your details')).toBeInTheDocument();
		const email = container.querySelector('input[name="giver_email"]');
		expect(email).toHaveAttribute('aria-required', 'true');
	});

	it('keeps the email field optional on a full-guest list', () => {
		render(Page, { data: listData(false), form: null });
		expect(screen.getByText('Email')).toBeInTheDocument();
		expect(screen.getByText('Your details (optional)')).toBeInTheDocument();
	});
});
