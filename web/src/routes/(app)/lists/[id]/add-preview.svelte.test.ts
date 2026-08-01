import { describe, it, expect, vi } from 'vitest';

// The list-detail add-item form uses superForm with resetForm:false so a ?/preview
// success keeps the scraped draft (the reset-on-success bug). That makes two server
// contracts load-bearing, both guarded here:
//   - ?/preview returns the scraped draft in form.data (the client keeps it), and
//   - ?/add on success returns a FRESH empty form so the form still clears after a
//     real add (since the client no longer resets it).
// backendClient is mocked so no real backend is needed; the preview path returns a
// scraped draft, and the last items-create POST / item PATCH bodies are captured so
// tests can assert what was sent (e.g. price on add/edit — #128 price editing).
const cap = vi.hoisted(() => ({ post: null as unknown, patch: null as unknown }));
vi.mock('$lib/server/api', () => ({
	backendClient: () => ({
		POST: async (path: string, opts?: { body?: { url?: string } }) => {
			if (path !== '/api/v1/item-previews') {
				cap.post = { path, body: opts?.body }; // items-create success
				return { error: undefined };
			}
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
		},
		PATCH: async (path: string, opts?: { body?: unknown }) => {
			cap.patch = { path, body: opts?.body };
			return { error: undefined };
		}
	}),
	backendPostRaw: async () => ({ status: 201, json: async () => ({ created: 0 }) })
}));

import { actions } from './+page.server';

const call = (name: 'add' | 'preview' | 'edit', fields: Record<string, string>) => {
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

describe('editable item price (#128 price editing)', () => {
	it('?/add sends price (uppercased currency) when an amount + currency are present', async () => {
		await call('add', {
			name: 'Item',
			quantity_wanted: '1',
			price_minor: '1500',
			price_currency: 'eur'
		});
		expect((cap.post as { body: { price?: unknown } }).body.price).toEqual({
			amount_minor: 1500,
			currency: 'EUR'
		});
	});

	it('?/add sends no price when the amount is blank', async () => {
		await call('add', {
			name: 'Item',
			quantity_wanted: '1',
			price_minor: '',
			price_currency: 'USD'
		});
		expect((cap.post as { body: { price?: unknown } }).body.price).toBeUndefined();
	});

	it('?/edit includes price:{amount_minor,currency} when an amount + currency are posted', async () => {
		await call('edit', {
			item_id: 'i1',
			name: 'Item',
			quantity_wanted: '1',
			price_amount: '19.99',
			price_currency: 'usd'
		});
		expect((cap.patch as { body: { price?: unknown } }).body.price).toEqual({
			amount_minor: 1999,
			currency: 'USD'
		});
	});

	it('?/edit omits price when the amount is blank', async () => {
		await call('edit', { item_id: 'i1', name: 'Item', quantity_wanted: '1', price_amount: '' });
		expect((cap.patch as { body: Record<string, unknown> }).body).not.toHaveProperty('price');
	});

	it('?/edit rejects an amount with no currency', async () => {
		const res = (await call('edit', {
			item_id: 'i1',
			name: 'Item',
			quantity_wanted: '1',
			price_amount: '9.99'
		})) as { status?: number; data?: { editError?: string } };
		expect(res.status).toBe(400);
	});
});
