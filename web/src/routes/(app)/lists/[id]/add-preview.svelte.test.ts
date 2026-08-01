import { describe, it, expect, vi } from 'vitest';

// The list-detail add-item form uses superForm with resetForm:false so a ?/preview
// success keeps the scraped draft (the reset-on-success bug). That makes two server
// contracts load-bearing, both guarded here:
//   - ?/preview returns the scraped draft in form.data (the client keeps it), and
//   - ?/add on success returns a FRESH empty form so the form still clears after a
//     real add (since the client no longer resets it).
// backendClient is mocked so no real backend is needed; the preview path returns a
// scraped draft, the items-create path returns success.
vi.mock('$lib/server/api', () => ({
	backendClient: () => ({
		POST: async (path: string, opts?: { body?: { url?: string } }) => {
			if (path !== '/api/v1/item-previews') return { error: undefined }; // items-create success
			// A url containing "nofetch" simulates a scrape that can't fetch the page.
			if (opts?.body?.url?.includes('nofetch')) return { data: undefined, error: { detail: 'x' } };
			return {
				data: {
					name: 'Scraped Widget',
					url: 'https://shop.example/widget',
					image_url: 'https://img.example/w.png',
					price: { amount_minor: 1999, currency: 'USD' }
				}
			};
		}
	}),
	backendPostRaw: async () => ({ status: 201, json: async () => ({ created: 0 }) })
}));

import { actions } from './+page.server';

const call = (name: 'add' | 'preview', fields: Record<string, string>) => {
	const fd = new FormData();
	for (const [k, v] of Object.entries(fields)) fd.set(k, v);
	const request = new Request('http://t.example/lists/l1?/' + name, { method: 'POST', body: fd });
	// Only the fields the actions actually read are needed on locals/params.
	return (
		actions[name] as (e: {
			request: Request;
			locals: { host: string; token: string };
			params: { id: string };
		}) => Promise<unknown>
	)({ request, locals: { host: 't.example', token: 'tok' }, params: { id: 'l1' } });
};

describe('add-item form server contracts (#128 preview-reset fix)', () => {
	it('?/preview returns the scraped draft in form.data', async () => {
		const res = (await call('preview', { url: 'https://paste.example/x' })) as {
			form: { data: Record<string, unknown> };
		};
		expect(res.form.data.name).toBe('Scraped Widget');
		expect(res.form.data.url).toBe('https://shop.example/widget');
		expect(res.form.data.image_url).toBe('https://img.example/w.png');
		expect(res.form.data.price_minor).toBe(1999);
		expect(res.form.data.price_currency).toBe('USD');
	});

	it('?/preview keeps the pasted url even when the scrape fails to fetch', async () => {
		// Invariant (2): when the page can't be fetched, the action returns the form with
		// the pasted url intact so the client (resetForm:false) keeps it for a manual add.
		const res = (await call('preview', { url: 'https://paste.example/nofetch' })) as {
			form: { data: Record<string, unknown> };
		};
		expect(res.form.data.url).toBe('https://paste.example/nofetch');
	});

	it('?/add on success returns a FRESH empty form so the form clears after add', async () => {
		const res = (await call('add', {
			name: 'Real Item',
			quantity_wanted: '1',
			url: 'https://x.example'
		})) as { addForm: { data: Record<string, unknown> } };
		// Before the fix this echoed the populated submitted form (name 'Real Item');
		// after, it is a fresh empty form so the client (resetForm:false) clears. name
		// is the discriminating field (fresh → ''); url is optional (fresh → undefined).
		expect(res.addForm.data.name).toBe('');
		expect(res.addForm.data.url).toBeFalsy();
	});
});
