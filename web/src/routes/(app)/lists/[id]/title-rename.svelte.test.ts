import { describe, it, expect, vi, beforeEach } from 'vitest';

// #404: the settings form can rename a list. The backend has accepted
// ListUpdate.title all along — the gap was that this form never sent it — so what
// is worth testing is not "the PATCH carries a title" on its own but the boundary
// the title does NOT share with the other text fields on the same form.
//
// A description is cleared by submitting it empty. A title has no valid empty
// state, and both arrive through the same `fd.get()` call, where an ABSENT field
// and a BLANK one are different values (null vs '') that the surrounding code
// collapses into ''. Collapsing them here would make the list's name clearable by
// the same path that clears a description, so the three cases — set, blank,
// absent — are asserted separately, and the description's own clearing behaviour
// is asserted alongside them to show the two did not converge.

let patched: Record<string, unknown> | undefined;
let patchCalls = 0;

vi.mock('$lib/server/api', () => ({
	backendClient: () => ({
		PATCH: async (_path: string, opts: { body: Record<string, unknown> }) => {
			patchCalls += 1;
			patched = opts.body;
			return { error: undefined };
		}
	}),
	backendPostRaw: async () => ({ error: undefined })
}));

vi.mock('$lib/server/markdown', () => ({ renderNote: (s: string) => s ?? '' }));

import { actions } from './+page.server';

// `patched` is only ever assigned inside the hoisted vi.mock factory, which
// TypeScript's control flow does not see; the annotation keeps it from narrowing
// to `undefined` (same reason as the sibling #308 test).
type Saved = { body: Record<string, unknown> | undefined; result: unknown; calls: number };

async function saveSettings(fields: Record<string, string>): Promise<Saved> {
	patched = undefined;
	patchCalls = 0;
	const fd = new FormData();
	for (const [k, v] of Object.entries(fields)) fd.append(k, v);
	const settings = actions.settings as unknown as (e: {
		request: { formData: () => Promise<FormData> };
		locals: { host: string; token: string };
		params: { id: string };
	}) => Promise<unknown>;
	const result = await settings({
		request: { formData: async () => fd },
		locals: { host: 't.example', token: 'tok' },
		params: { id: 'l1' }
	});
	return { body: patched, result, calls: patchCalls };
}

// A superforms/kit `fail` carries its payload under `data`; narrow to the fields
// this action returns rather than asserting on an `unknown`.
function failure(result: unknown): { status?: number; data?: { settingsError?: string } } {
	return result as { status?: number; data?: { settingsError?: string } };
}

describe('renaming a list from settings (#404)', () => {
	beforeEach(() => {
		patched = undefined;
		patchCalls = 0;
	});

	it('sends a changed title on the settings PATCH', async () => {
		const { body } = await saveSettings({ title: 'Wedding gifts', description: '' });
		expect(body?.title).toBe('Wedding gifts');
	});

	it('trims surrounding whitespace before sending', async () => {
		const { body } = await saveSettings({ title: '  Housewarming  ' });
		expect(body?.title).toBe('Housewarming');
	});

	it('refuses a blank title instead of clearing the name', async () => {
		const { result, calls } = await saveSettings({ title: '', description: '' });
		expect(failure(result).status).toBe(400);
		expect(failure(result).data?.settingsError).toBe('Title is required.');
		// The refusal has to happen before the request, not be repaired after it: a
		// PATCH that went out with an empty title would already have renamed the list.
		expect(calls).toBe(0);
	});

	it('refuses a whitespace-only title', async () => {
		const { result, calls } = await saveSettings({ title: '   ' });
		expect(failure(result).status).toBe(400);
		expect(calls).toBe(0);
	});

	// The distinction the implementation turns on. A form that carries no title input
	// is not an owner clearing the title, and must not be refused — nor may it send
	// `title: ''`, which the merge-patch backend would apply as a rename to empty.
	it('leaves the title untouched when the form carries no title field', async () => {
		const { body, result, calls } = await saveSettings({ description: 'still here' });
		expect(calls).toBe(1);
		expect(body).not.toHaveProperty('title');
		expect(failure(result).status).toBeUndefined();
	});

	// Guards the boundary from the other side: the blank-is-refused rule is the
	// title's alone, and must not have been generalised onto the fields whose empty
	// state is meaningful.
	it('still clears a description submitted empty', async () => {
		const { body, calls } = await saveSettings({ title: 'Keep me', description: '' });
		expect(calls).toBe(1);
		expect(body?.description).toBe('');
	});
});
