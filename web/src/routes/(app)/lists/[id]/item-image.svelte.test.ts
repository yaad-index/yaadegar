import { describe, it, expect, vi, beforeEach } from 'vitest';

// #414: an item's image could only ever come from a successful scrape. The field was
// a hidden input the preview alone wrote, and the edit form had no image control at
// all, so a failed scrape — the common case — left an item nobody could finish by
// hand. These cases cover the manual path that was missing, and the reporting that
// tells a failed scrape apart from one that was never run.
//
// The scrape is mocked: a url containing "noimage" fetches but has no image, and one
// containing "nofetch" fails outright.
const cap = vi.hoisted(() => ({ post: null as unknown, patch: null as unknown }));
vi.mock('$lib/server/api', () => ({
	backendClient: () => ({
		POST: async (path: string, opts?: { body?: { url?: string } }) => {
			if (path !== '/api/v1/item-previews') {
				cap.post = { path, body: opts?.body };
				return { error: undefined };
			}
			const url = opts?.body?.url ?? '';
			if (url.includes('nofetch')) return { data: undefined, error: { detail: 'x' } };
			if (url.includes('nothing')) return { data: {} };
			if (url.includes('noimage')) {
				return { data: { name: 'Scraped Widget', price: { amount_minor: 1999, currency: 'USD' } } };
			}
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

vi.mock('$lib/server/markdown', () => ({ renderNote: (s: string) => s ?? '' }));

import { actions } from './+page.server';

const call = (name: 'add' | 'preview' | 'edit', fields: Record<string, string>) => {
	const fd = new FormData();
	for (const [k, v] of Object.entries(fields)) fd.set(k, v);
	const request = new Request('http://t.example/lists/l1?/' + name, { method: 'POST', body: fd });
	return (
		actions[name] as (e: {
			request: Request;
			locals: { host: string; token: string };
			params: { id: string };
		}) => Promise<unknown>
	)({ request, locals: { host: 't.example', token: 'tok' }, params: { id: 'l1' } });
};

const postBody = () => (cap.post as { body?: Record<string, unknown> } | null)?.body;
const patchBody = () => (cap.patch as { body?: Record<string, unknown> } | null)?.body;
const msg = (res: unknown) => (res as { form?: { message?: string } })?.form?.message;

beforeEach(() => {
	cap.post = null;
	cap.patch = null;
});

describe('setting an item image by hand (#414)', () => {
	it('sends an image typed into the add form, with no scrape involved', async () => {
		await call('add', {
			name: 'Hand-made',
			quantity_wanted: '1',
			image_url: 'https://img.example/typed.png'
		});
		expect(postBody()?.image_url).toBe('https://img.example/typed.png');
	});

	it('sends an image typed into the edit form', async () => {
		await call('edit', {
			item_id: 'i1',
			name: 'Item',
			quantity_wanted: '1',
			image_url: 'https://img.example/edited.png'
		});
		expect(patchBody()?.image_url).toBe('https://img.example/edited.png');
	});

	// A scrape guesses, so it can guess wrong. Replacing the image was never the
	// problem; getting back to none was impossible.
	it('clears the image when the edit box is emptied', async () => {
		await call('edit', { item_id: 'i1', name: 'Item', quantity_wanted: '1', image_url: '' });
		expect(patchBody()).toHaveProperty('image_url', '');
	});

	// Absent is not blank — the same distinction the list-title rule turns on. A form
	// that never carried the input is some other caller, and clearing an image it did
	// not know about would be a write nobody asked for.
	it('leaves the image alone when the edit form carries no image field', async () => {
		await call('edit', { item_id: 'i1', name: 'Item', quantity_wanted: '1' });
		expect(patchBody()).not.toHaveProperty('image_url');
	});
});

describe('telling a failed scrape from one that was never run (#414)', () => {
	it('names what it filled in when the scrape works', async () => {
		const res = await call('preview', { url: 'https://paste.example/x' });
		expect(msg(res)).toMatch(/image/);
		expect(msg(res)).toMatch(/review and add/);
	});

	// The case the issue is about: the fetch succeeded, there was simply no image, and
	// the empty box previously looked identical to "never fetched".
	it('says so when the page had no image', async () => {
		const res = await call('preview', { url: 'https://paste.example/noimage' });
		expect(msg(res)).toMatch(/no image/i);
		expect(msg(res)).toMatch(/name/);
	});

	it('says the fetch failed when the page could not be fetched', async () => {
		const res = await call('preview', { url: 'https://paste.example/nofetch' });
		expect(msg(res)).toMatch(/couldn't fetch/i);
	});

	it('says so when the page was fetched but yielded nothing', async () => {
		const res = await call('preview', { url: 'https://paste.example/nothing' });
		expect(msg(res)).toMatch(/nothing to fill in/i);
	});

	// The fetch fills gaps; it must not undo manual work. A scrape with no image
	// used to blank whatever the owner had already typed.
	it('does not wipe a typed image when the scrape finds none', async () => {
		const res = (await call('preview', {
			url: 'https://paste.example/noimage',
			image_url: 'https://img.example/mine.png'
		})) as { form: { data: Record<string, unknown> } };
		expect(res.form.data.image_url).toBe('https://img.example/mine.png');
	});
});
