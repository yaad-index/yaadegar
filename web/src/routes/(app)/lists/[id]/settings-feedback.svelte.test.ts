import { describe, it, expect, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/svelte';
import { page } from '$app/state';
import Page from './+page.svelte';
import type { PageData, ActionData } from './$types';

// #313: the ?/settings action has always returned settingsError on a rejected save
// and settingsSaved on a successful one, and the page read neither — so a save the
// server refused left the refused value sitting in the control with nothing anywhere
// on the page to say it had not taken.
//
// These assertions are about what a person can SEE after a save, which is the
// property the issue is about. Asserting that the action returns the field would not
// be that property: it returned the field the whole time it was broken.

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
		// superForm() owns this store, so it has to be a real shape rather than a cast
		// past it — the component constructs the store at the top level on every render.
		addForm: { id: 'add', valid: true, posted: false, errors: {}, data: {} }
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
	}) as any;

const renderSettingsTab = (form: ActionData) => render(Page, { props: { data: listData(), form } });

describe('list settings save feedback (#313)', () => {
	beforeEach(() => {
		// The page picks its tab off the URL, and the settings form only exists on the
		// settings tab — without this every assertion below would pass or fail on a
		// page that never rendered the form at all.
		// Cast: SvelteKit types page.url's pathname as the union of real routes, which
		// a plain URL does not satisfy. Only the query string matters here.
		page.url = new URL('http://test.local/lists/l1?tab=settings') as typeof page.url;
	});

	it('shows the error next to the form when the server rejects the save', () => {
		renderSettingsTab({ settingsError: 'Could not update list settings.' } as ActionData);
		const alert = screen.getByRole('alert');
		expect(alert).toHaveTextContent('Could not update list settings.');
	});

	it('confirms a save that succeeded', () => {
		renderSettingsTab({ settingsSaved: true } as ActionData);
		expect(screen.getByRole('status')).toHaveTextContent('Settings saved.');
	});

	// Without this, both assertions above would still pass if the page rendered both
	// messages unconditionally — they would be reading permanent furniture rather than
	// a result. This is the case that makes them mean something.
	it('says nothing before a save has been attempted', () => {
		renderSettingsTab(null);
		expect(screen.queryByRole('alert')).toBeNull();
		expect(screen.queryByRole('status')).toBeNull();
	});

	// The settings form is on the same tab as the import form, which has its own
	// error/success lines. A result belonging to one form must not light up the other.
	it('does not report a settings outcome for an import result', () => {
		renderSettingsTab({ importError: 'Could not import that file.' } as ActionData);
		expect(screen.queryByText('Settings saved.')).toBeNull();
		expect(screen.getByRole('alert')).toHaveTextContent('Could not import that file.');
	});
});
