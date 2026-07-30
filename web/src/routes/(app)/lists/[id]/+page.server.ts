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
	// client-side. The preview validates ONLY that a link was pasted — it POPULATES
	// name/url/image/price, so it must not fail on an empty name/quantity (that would
	// make superforms return a failure and the client discard the scraped prefill,
	// #79 follow-up). The returned form is always success so enhance applies the
	// fetched values; ?/add still does the real name validation.
	preview: async ({ request, locals }) => {
		const form = await superValidate(request, zod4(addItemSchema));
		form.valid = true;
		form.errors = {};
		const link = (form.data.url || '').trim();
		if (!link) return message(form, 'Paste a product link first.');
		const client = backendClient({ host: locals.host, token: locals.token });
		const { data, error: err } = await client.POST('/api/v1/item-previews', {
			body: { url: link }
		});
		if (err || !data) {
			return message(form, "Couldn't fetch that page — enter the details manually.");
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
		// Per-item co-buy override (#100/#111): 'true'/'false' set it; '' ("use list
		// default") sends explicit null, which clears the override back to inheriting
		// the list default (three-state PATCH).
		const allowCobuyRaw = String(fd.get('allow_cobuy') ?? '');
		const allow_cobuy = allowCobuyRaw === 'true' ? true : allowCobuyRaw === 'false' ? false : null;
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
				note: note || undefined,
				allow_cobuy
			}
		});
		if (err) return fail(400, { editError: 'Could not update the item.' });
		return { edited: true };
	},

	// List-level co-buy default (#100): items with no override inherit it.
	settings: async ({ request, locals, params }) => {
		const fd = await request.formData();
		const allow_cobuy = String(fd.get('allow_cobuy') ?? 'true') === 'true';
		const client = backendClient({ host: locals.host, token: locals.token });
		const { error: err } = await client.PATCH('/api/v1/lists/{listId}', {
			params: { path: { listId: params.id } },
			body: { allow_cobuy }
		});
		if (err) return fail(400, { settingsError: 'Could not update list settings.' });
		return { settingsSaved: true };
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
