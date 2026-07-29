import { error, fail } from '@sveltejs/kit';
import { superValidate, message } from 'sveltekit-superforms';
import { zod4 } from 'sveltekit-superforms/adapters';
import { z } from 'zod';
import { backendClient } from '$lib/server/api';
import { renderNote } from '$lib/server/markdown';
import type { Actions, PageServerLoad } from './$types';

const addItemSchema = z.object({
	name: z.string().min(1, 'Name is required'),
	url: z.string().url('Must be a valid URL').optional().or(z.literal('')),
	note: z.string().max(4000).optional(),
	quantity_wanted: z.coerce.number().int().min(1).default(1),
	// Carried through from a URL preview (the ?/preview action) so a scraped image
	// and price ride along into the create; not directly typed by the owner.
	image_url: z.string().optional(),
	price_minor: z.coerce.number().int().optional(),
	price_currency: z.string().optional()
});

export const load: PageServerLoad = async ({ locals, params }) => {
	const client = backendClient({ host: locals.host, token: locals.token });
	const [listRes, itemsRes] = await Promise.all([
		client.GET('/api/v1/lists/{listId}', { params: { path: { listId: params.id } } }),
		client.GET('/api/v1/lists/{listId}/items', {
			params: { path: { listId: params.id }, query: { limit: 200 } }
		})
	]);
	if (listRes.error || !listRes.data) {
		error(listRes.response.status || 404, 'List not found.');
	}
	const items = itemsRes.data?.items ?? [];
	return {
		list: listRes.data,
		items,
		// Notes are rendered to sanitized HTML server-side; {@html} only ever touches
		// this map, never a raw note (ADR-0006 security boundary).
		noteHtml: Object.fromEntries(items.map((i) => [i.id ?? '', renderNote(i.note)])),
		addForm: await superValidate(zod4(addItemSchema))
	};
};

export const actions: Actions = {
	add: async ({ request, locals, params }) => {
		const form = await superValidate(request, zod4(addItemSchema));
		if (!form.valid) return fail(400, { addForm: form });
		const client = backendClient({ host: locals.host, token: locals.token });
		const { error: err } = await client.POST('/api/v1/lists/{listId}/items', {
			params: { path: { listId: params.id } },
			body: {
				name: form.data.name,
				url: form.data.url || undefined,
				note: form.data.note || undefined,
				quantity_wanted: form.data.quantity_wanted,
				image_url: form.data.image_url || undefined,
				price:
					form.data.price_minor != null && form.data.price_currency
						? { amount_minor: form.data.price_minor, currency: form.data.price_currency }
						: undefined,
				// priority isn't surfaced in the UI; send the default (the spec marks it
				// required, though the backend defaults it too).
				priority: 0
			}
		});
		if (err) return message(form, 'Could not add the item.', { status: 400 });
		return { addForm: form };
	},

	// Auto-fill from a pasted product URL by reusing the SSRF-safe preview endpoint
	// (#10) server-side; the owner reviews the draft before adding. Never scrapes
	// client-side.
	preview: async ({ request, locals }) => {
		const form = await superValidate(request, zod4(addItemSchema));
		const url = (form.data.url || '').trim();
		if (!url) return message(form, 'Paste a product link first.', { status: 400 });
		const client = backendClient({ host: locals.host, token: locals.token });
		const {
			data,
			error: err,
			response
		} = await client.POST('/api/v1/item-previews', {
			body: { url }
		});
		if (err || !data) {
			return message(form, "Couldn't fetch that page — enter the details manually.", {
				status: response?.status === 422 ? 422 : 400
			});
		}
		if (data.name) form.data.name = data.name;
		if (data.url) form.data.url = data.url;
		form.data.image_url = data.image_url ?? undefined;
		form.data.price_minor = data.price?.amount_minor ?? undefined;
		form.data.price_currency = data.price?.currency ?? undefined;
		return message(form, 'Fetched — review and add.');
	},

	edit: async ({ request, locals }) => {
		const fd = await request.formData();
		const itemId = String(fd.get('item_id') ?? '');
		const name = String(fd.get('name') ?? '').trim();
		const url = String(fd.get('url') ?? '').trim();
		const note = String(fd.get('note') ?? '').trim();
		const quantity = Number(fd.get('quantity_wanted') ?? 1);
		if (!itemId || !name) return fail(400, { editError: 'Name is required.' });
		const client = backendClient({ host: locals.host, token: locals.token });
		// The item PATCH is set-if-present; send only the fields with values (clearing a
		// field back to empty is not supported by the current backend semantics).
		const { error: err } = await client.PATCH('/api/v1/items/{itemId}', {
			params: { path: { itemId } },
			body: {
				name,
				quantity_wanted: Number.isFinite(quantity) ? quantity : 1,
				url: url || undefined,
				note: note || undefined
			}
		});
		if (err) return fail(400, { editError: 'Could not update the item.' });
		return { edited: true };
	},

	delete: async ({ request, locals }) => {
		const fd = await request.formData();
		const itemId = String(fd.get('item_id') ?? '');
		if (!itemId) return fail(400, {});
		const client = backendClient({ host: locals.host, token: locals.token });
		const { error: err } = await client.DELETE('/api/v1/items/{itemId}', {
			params: { path: { itemId } }
		});
		if (err) return fail(400, { deleteError: 'Could not delete the item.' });
		return { deleted: true };
	}
};
