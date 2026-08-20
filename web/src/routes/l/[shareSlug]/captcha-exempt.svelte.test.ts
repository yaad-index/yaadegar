import { describe, it, expect, vi } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import Page from './+page.svelte';
import type { PageData } from './$types';

// The Altcha widget is a real browser custom element jsdom cannot run; stub the
// dynamic import so the page template still renders (same rationale as the widget
// gating test). These cases only exercise the *disabled* wiring, not a live solve.
vi.mock('altcha', () => ({}));

// A low-trust guest page in the blocking state: a captcha provider is configured but
// no token has been solved yet (captchaToken starts empty), so captchaBlocking is
// true — exactly the post-reserve reset state #246 was about. `turnstile` with no
// site key keeps captchaBlocking true (it reads data.captchaProvider) without needing
// the widget to actually mount. Items cover every action: an available item (Reserve
// + chip-in), a reserved-by-this-browser item (Release), and a pledged item (Withdraw
// + refresh).
function blockingPage(): PageData {
	return {
		closed: false,
		list: {
			title: 'Birthday',
			email_required: false,
			account_required: false,
			items: [
				{
					id: 'a',
					name: 'Available gift',
					availability: 'available',
					price: { amount_minor: 2000, currency: 'EUR' },
					allow_cobuy: true
				},
				{ id: 'b', name: 'Reserved gift', availability: 'reserved' },
				{ id: 'c', name: 'Pledged gift', availability: 'co_buying' }
			]
		},
		reservedItemIds: ['b'],
		pledged: {
			c: { contribution_id: 'x1', status: 'pending', matched: false, match_id: null }
		},
		noteHtml: {},
		descriptionHtml: '',
		accountRequired: false,
		loggedIn: false,
		registrationEnabled: false,
		captchaProvider: 'turnstile',
		captchaSiteKey: '',
		reservePath: '',
		shareSlug: 's1'
	} as unknown as PageData;
}

// These assertions fail the moment an exempt action becomes inert again (the #246
// failure the exemption removes). Two independent gates had to be cleared, so both are
// asserted: the button must be ENABLED (the disabled gate removed) AND carry
// `formnovalidate`. The second is load-bearing and easy to drop by accident: altcha
// injects a `required` checkbox into the shared form, so while unverified the form is
// invalid and native constraint validation silently swallows every submit — an enabled
// Release button with no formnovalidate would still do nothing (verified by driving a
// real widget; jsdom mocks altcha so it cannot reproduce the block here, which is
// exactly why this structural assertion stands in for it).
describe('anti-bot exemption for undo/read actions (#249)', () => {
	it('leaves Release enabled and validation-exempt while unverified', () => {
		render(Page, { data: blockingPage(), form: null });
		const btn = screen.getByRole('button', { name: 'Release' });
		expect(btn).toBeEnabled();
		expect(btn).toHaveAttribute('formnovalidate');
	});

	it('leaves Withdraw pledge enabled and validation-exempt while unverified', () => {
		render(Page, { data: blockingPage(), form: null });
		const btn = screen.getByRole('button', { name: 'Withdraw pledge' });
		expect(btn).toBeEnabled();
		expect(btn).toHaveAttribute('formnovalidate');
	});

	it('leaves the refresh ("Check for updates") read enabled and validation-exempt', () => {
		render(Page, { data: blockingPage(), form: null });
		const btn = screen.getByRole('button', { name: 'Check for updates' });
		expect(btn).toBeEnabled();
		expect(btn).toHaveAttribute('formnovalidate');
	});

	// The other half of the decision: the guard must STAY on the state-creating actions.
	// If a change accidentally exempted reserve/pledge too, these fail.
	it('keeps Reserve disabled and validated while unverified (guard stays)', () => {
		render(Page, { data: blockingPage(), form: null });
		const btn = screen.getByRole('button', { name: 'Reserve it' });
		expect(btn).toBeDisabled();
		// Reserve must NOT be validation-exempt — it is a state-creating action the guard
		// is for, so it keeps the shared form's constraint validation.
		expect(btn).not.toHaveAttribute('formnovalidate');
	});

	it('keeps Pledge disabled while unverified, with a visible note', async () => {
		render(Page, { data: blockingPage(), form: null });
		// Open the chip-in form on the available item (priced + allow_cobuy).
		await fireEvent.click(screen.getByRole('button', { name: 'Chip in instead' }));
		expect(screen.getByRole('button', { name: 'Pledge' })).toBeDisabled();
		// A disabled control must explain itself — no silent refusal (#246/#247).
		expect(screen.getAllByText('Complete the anti-bot check above first.').length).toBeGreaterThan(
			0
		);
	});
});
