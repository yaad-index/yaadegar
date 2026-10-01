import { describe, it, expect } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, within } from '@testing-library/svelte';
import Page from './+page.svelte';
import type { PageData, ActionData } from './$types';

// #397 (ADR-0016 §4, §7): what a person can see in the Access tokens section of
// Settings — the new token shown once, each token's state and dates, the way back
// in when creation needs a recent sign-in, and the reset and password-change notes.

const token = (over: Record<string, unknown>) => ({
	id: 't1',
	name: 'Backup script',
	last4: 'Ab12',
	created_at: '2027-06-01T10:00:00Z',
	expires_at: null,
	last_used_at: null,
	revoked_at: null,
	active: true,
	...over
});

const data = (over: Record<string, unknown> = {}): PageData =>
	({
		settings: { oauth_google_enabled: false, google_client_configured: false },
		domains: [],
		ownerKey: null,
		tokens: [],
		tokenExpiryDays: [30, 90, 365],
		tokensRevoked: 0,
		user: { name: 'Alice' },
		...over
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
	}) as any;

const section = () => within(document.getElementById('access-tokens') as HTMLElement);

describe('Settings access tokens (ADR-0016)', () => {
	it('offers every expiry and preselects none of them', () => {
		render(Page, { props: { data: data(), form: null as ActionData } });
		const select = section().getByRole('combobox') as HTMLSelectElement;
		expect(select).toBeRequired();
		expect(select.value).toBe('');
		const options = within(select)
			.getAllByRole('option')
			.map((o) => o.textContent?.trim());
		expect(options).toEqual(['Choose…', 'In 30 days', 'In 90 days', 'In 365 days', 'Never']);
		expect(section().getByText('No access tokens yet.')).toBeInTheDocument();
	});

	it('shows a new token once, ready to copy, and says it will not be shown again', () => {
		const form = { createdToken: 'ydg_pat_secretvalue', createdTokenName: 'CI' } as ActionData;
		render(Page, { props: { data: data(), form } });
		expect(section().getByLabelText('Your new access token')).toHaveValue('ydg_pat_secretvalue');
		expect(section().getByText(/won't be able to see it again/)).toBeInTheDocument();
		expect(section().getByRole('button', { name: 'Copy' })).toBeInTheDocument();
	});

	it('does not show any token value without a fresh creation', () => {
		render(Page, { props: { data: data({ tokens: [token({})] }), form: null as ActionData } });
		expect(section().queryByLabelText('Your new access token')).toBeNull();
	});

	it('lists each token with its state, dates and the last four characters', () => {
		const tokens = [
			token({ id: 'a', name: 'Active one', last_used_at: '2027-06-10T08:00:00Z' }),
			token({
				id: 'b',
				name: 'Revoked one',
				active: false,
				revoked_at: '2027-06-05T00:00:00Z',
				expires_at: '2027-09-01T00:00:00Z'
			}),
			token({ id: 'c', name: 'Expired one', active: false, expires_at: '2027-06-02T00:00:00Z' })
		];
		render(Page, { props: { data: data({ tokens }), form: null as ActionData } });
		const items = section().getAllByRole('listitem');
		expect(items).toHaveLength(3);
		expect(within(items[0]).getByText('Active')).toBeInTheDocument();
		expect(within(items[0]).getByText('…Ab12')).toBeInTheDocument();
		expect(items[0]).toHaveTextContent(
			'Created 1 Jun 2027 · Never expires · Last used 10 Jun 2027'
		);
		expect(within(items[0]).getByRole('button', { name: 'Revoke' })).toBeInTheDocument();
		expect(within(items[1]).getByText('Revoked')).toBeInTheDocument();
		expect(items[1]).toHaveTextContent('Expires 1 Sept 2027');
		expect(within(items[1]).queryByRole('button', { name: 'Revoke' })).toBeNull();
		expect(within(items[2]).getByText('Expired')).toBeInTheDocument();
		expect(within(items[2]).queryByRole('button', { name: 'Revoke' })).toBeNull();
		expect(section().getByText('1 of 20 active tokens.')).toBeInTheDocument();
	});

	it('offers to sign in again when creation needs a recent sign-in', () => {
		const form = {
			tokenError: 'creating a token needs a recent sign-in; sign in again and retry',
			tokenNeedsSignIn: true
		} as ActionData;
		const { container } = render(Page, { props: { data: data(), form } });
		const button = section().getByRole('button', { name: 'Sign in again' });
		expect(button.closest('form')).toHaveAttribute('action', '/logout?return_to=/settings');
		expect(container).toHaveTextContent('recent sign-in');
	});

	it('does not offer to sign in again for other refusals', () => {
		const form = {
			tokenError: 'this account already has 20 active tokens',
			tokenNeedsSignIn: false
		} as ActionData;
		render(Page, { props: { data: data(), form } });
		expect(section().queryByRole('button', { name: 'Sign in again' })).toBeNull();
	});

	it('says how many tokens a password reset revoked', () => {
		render(Page, { props: { data: data({ tokensRevoked: 3 }), form: null as ActionData } });
		expect(section().getByText(/all 3 of your access tokens were revoked/)).toBeInTheDocument();
	});

	it('points at the tokens that still work after a password change', () => {
		const form = { passwordChanged: true, activeTokens: 2 } as ActionData;
		render(Page, { props: { data: data(), form } });
		expect(screen.getByText(/Your 2 access tokens still work/)).toBeInTheDocument();
		expect(screen.getByRole('link', { name: 'Review them' })).toHaveAttribute(
			'href',
			'#access-tokens'
		);
	});

	it('says nothing about tokens after a password change when the count is unknown', () => {
		const form = { passwordChanged: true, activeTokens: null } as ActionData;
		render(Page, { props: { data: data(), form } });
		expect(screen.getByText('Password changed.')).toBeInTheDocument();
		expect(screen.queryByRole('link', { name: 'Review them' })).toBeNull();
	});
});
