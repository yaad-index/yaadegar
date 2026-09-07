import { describe, it, expect, vi, beforeEach } from 'vitest';

// #308: the "Show on my shared page" control has two positions, but the stored
// visibility enum has three values. The mapping is the whole of the logic, and its
// interesting case is the one the UI cannot show: turning the control OFF on a list
// that was `unlisted` must leave it `unlisted`, not rewrite it to `private`. Both
// non-listed values behave identically under this feature, so a rewrite would be
// invisible in the UI while quietly changing stored state.

let patched: Record<string, unknown> | undefined;

vi.mock('$lib/server/api', () => ({
	backendClient: () => ({
		PATCH: async (_path: string, opts: { body: Record<string, unknown> }) => {
			patched = opts.body;
			return { error: undefined };
		}
	}),
	backendPostRaw: async () => ({ error: undefined })
}));

vi.mock('$lib/server/markdown', () => ({ renderNote: (s: string) => s ?? '' }));

import { actions } from './+page.server';

// The return type is annotated rather than inferred: `patched` is only ever assigned
// from inside the hoisted vi.mock factory, which TypeScript's control flow does not
// see, so it would otherwise narrow to `undefined` and make every field access an
// error on `never`.
async function saveSettings(
	fields: Record<string, string>
): Promise<Record<string, unknown> | undefined> {
	patched = undefined;
	const fd = new FormData();
	for (const [k, v] of Object.entries(fields)) fd.append(k, v);
	const settings = actions.settings as unknown as (e: {
		request: { formData: () => Promise<FormData> };
		locals: { host: string; token: string };
		params: { id: string };
	}) => Promise<unknown>;
	await settings({
		request: { formData: async () => fd },
		locals: { host: 't.example', token: 'tok' },
		params: { id: 'l1' }
	});
	return patched;
}

describe('owner-page inclusion toggle (#308)', () => {
	beforeEach(() => {
		patched = undefined;
	});

	it('turning it on lists the list', async () => {
		const body = await saveSettings({ current_visibility: 'private', listed: 'on' });
		expect(body?.visibility).toBe('public');
	});

	it('turning it off on a listed list unlists it', async () => {
		const body = await saveSettings({ current_visibility: 'public' });
		expect(body?.visibility).toBe('private');
	});

	it('leaving it off does not rewrite an unlisted list to private', async () => {
		// The case with no visible symptom: both values keep the list off the shared
		// page, so a rewrite here would never show up in the UI.
		const body = await saveSettings({ current_visibility: 'unlisted' });
		expect(body?.visibility).toBe('unlisted');
	});

	it('leaving it off leaves a private list private', async () => {
		const body = await saveSettings({ current_visibility: 'private' });
		expect(body?.visibility).toBe('private');
	});

	it('still sends the other list settings alongside it', async () => {
		const body = await saveSettings({
			current_visibility: 'private',
			listed: 'on',
			allow_cobuy: 'false',
			description: 'hello',
			reserver_tier: ''
		});
		expect(body?.allow_cobuy).toBe(false);
		expect(body?.description).toBe('hello');
		expect(body?.reserver_tier).toBeNull();
	});
});
