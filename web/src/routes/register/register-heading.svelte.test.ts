import { describe, it, expect } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';
import type { PageData } from './$types';

// When self-registration is disabled the page shows a not-enabled notice instead of the
// form, so its title and heading must stop naming the create action — otherwise
// "Create an account" sits above "Registration isn't enabled" and reads as a broken
// page. The heading/title track the same closed state the notice branch uses.
function pageData(over: Partial<PageData>): PageData {
	return { returnTo: '', registrationEnabled: true, ...over } as unknown as PageData;
}

describe('register page heading/title vs the disabled state', () => {
	it('names the create action and shows the form when registration is enabled', () => {
		render(Page, { data: pageData({ registrationEnabled: true }), form: null });
		expect(screen.getByRole('heading', { name: 'Create an account' })).toBeInTheDocument();
		expect(screen.getByRole('button', { name: 'Create account' })).toBeInTheDocument();
		expect(document.title).toBe('Create an account · Yaadegar');
	});

	it('reads "Registration" with the notice, not the form, when registration is disabled', () => {
		render(Page, { data: pageData({ registrationEnabled: false }), form: null });
		expect(screen.getByRole('heading', { name: 'Registration' })).toBeInTheDocument();
		expect(screen.queryByRole('heading', { name: 'Create an account' })).not.toBeInTheDocument();
		expect(screen.queryByRole('button', { name: 'Create account' })).not.toBeInTheDocument();
		expect(screen.getByText(/registration isn't enabled/i)).toBeInTheDocument();
		expect(document.title).toBe('Registration · Yaadegar');
	});
});
