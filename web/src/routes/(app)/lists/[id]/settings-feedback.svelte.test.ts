import { describe, it, expect, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
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

// #337: the banner outlived the state it described. SvelteKit keeps the `form` prop
// until the next action result or a navigation, so "Settings saved." stayed on screen
// while the user edited a control — and the page then rendered a saved state and an
// unsaved one identically, which is the same harm as showing nothing at all.
//
// Asserted on what a person can SEE after an edit, for the same reason as above: the
// action returned the field correctly the whole time the page was misleading.
//
// Queried by text rather than by role: selecting "Require an account" on an instance
// with registration disabled renders its own role="status" warning, so a role query
// would be ambiguous in exactly the case worth testing.
describe('list settings feedback does not outlive the state it describes (#337)', () => {
	beforeEach(() => {
		page.url = new URL('http://test.local/lists/l1?tab=settings') as typeof page.url;
	});

	const renderTab = (form: ActionData) => render(Page, { props: { data: listData(), form } });

	it('drops the success banner when a text control is edited', async () => {
		const { container } = renderTab({ settingsSaved: true } as ActionData);
		expect(screen.getByText('Settings saved.')).toBeInTheDocument();

		await fireEvent.input(container.querySelector('#list-description')!, {
			target: { value: 'A note for givers' }
		});

		expect(screen.queryByText('Settings saved.')).toBeNull();
	});

	// The control the issue names. A select emits change rather than input, so this
	// covers the other half of the pair the form listens for — without it, a handler
	// bound to input alone would pass every other assertion in this block.
	it('drops the success banner when the reserver tier is changed', async () => {
		const { container } = renderTab({ settingsSaved: true } as ActionData);
		expect(screen.getByText('Settings saved.')).toBeInTheDocument();

		await fireEvent.change(container.querySelector('#list-reserver-tier')!, {
			target: { value: 'registered' }
		});

		expect(screen.queryByText('Settings saved.')).toBeNull();
	});

	// Deliberately NOT symmetrical with the success case. A stale success understates a
	// problem and removes the reason to look further; a stale error overstates one and
	// is the very thing being edited in response to, so clearing it on the first
	// keystroke would delete the instruction while it is being followed.
	it('keeps the error while the user edits, because the error is what they are acting on', async () => {
		const { container } = renderTab({
			settingsError: 'Could not update list settings.'
		} as ActionData);

		await fireEvent.input(container.querySelector('#list-description')!, {
			target: { value: 'trying again' }
		});

		expect(screen.getByRole('alert')).toHaveTextContent('Could not update list settings.');
	});

	it('drops the import banner when a different file is chosen', async () => {
		const { container } = renderTab({ imported: 12 } as ActionData);
		expect(screen.getByText('Imported 12 item(s).')).toBeInTheDocument();

		await fireEvent.change(container.querySelector('input[type="file"]')!);

		expect(screen.queryByText('Imported 12 item(s).')).toBeNull();
	});

	// The one that separates this implementation from a boolean "edited" flag. The
	// dismissal is held by result identity, so the NEXT save is new feedback rather
	// than something a latched flag keeps suppressed. A flag that nothing resets would
	// pass every other assertion here and silently break the feature on the second
	// save — the harder failure to notice, because by then the page looks broken in
	// the direction people expect to be safe.
	it('shows the banner again for the next save, so dismissal is not a latch', async () => {
		const { container, rerender } = renderTab({ settingsSaved: true } as ActionData);

		await fireEvent.input(container.querySelector('#list-description')!, {
			target: { value: 'edited after saving' }
		});
		expect(screen.queryByText('Settings saved.')).toBeNull();

		// A fresh action result: a new object, as SvelteKit delivers on the next save.
		await rerender({ data: listData(), form: { settingsSaved: true } as ActionData });

		expect(screen.getByText('Settings saved.')).toBeInTheDocument();
	});

	// The two blocks keep their own dismissal, so editing one does not silently
	// acknowledge the other's result on the user's behalf. Sequenced rather than
	// staged as one combined result: ActionData is a union of mutually exclusive
	// shapes, so an object carrying both a settings and an import outcome is a state
	// the action cannot return, and asserting against it would prove nothing about
	// anything reachable. Typing this as a single result is rejected by svelte-check,
	// which is how the impossible version was caught.
	//
	// The import banner SHOULD survive here: an import that happened is not made
	// untrue by editing a setting, so a shared dismissal would wrongly clear it.
	it('editing the settings form leaves the import banner alone', async () => {
		const { container } = renderTab({ imported: 12 } as ActionData);
		expect(screen.getByText('Imported 12 item(s).')).toBeInTheDocument();

		await fireEvent.input(container.querySelector('#list-description')!, {
			target: { value: 'edited' }
		});

		expect(screen.getByText('Imported 12 item(s).')).toBeInTheDocument();
	});
});
