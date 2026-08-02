import { error, fail } from '@sveltejs/kit';
import { superValidate, message } from 'sveltekit-superforms';
import { zod4 } from 'sveltekit-superforms/adapters';
import { z } from 'zod';
import { backendClient, backendPostRaw } from '$lib/server/api';
import { renderNote } from '$lib/server/markdown';
import type { Actions, PageServerLoad } from './$types';

const addItemSchema = z.object({
	name: z.string().min(1, 'Name is required'),
	url: z.string().url('Must be a valid URL').optional().or(z.literal('')),
	note: z.string().max(4000).optional(),
	quantity_wanted: z.coerce.number().int().min(1).default(1),
	// image_url rides along from a URL preview (scraped, not directly typed).
	image_url: z.string().optional(),
	// price_minor is driven by the owner-editable price amount (major units → minor,
	// #128 price editing); an empty amount submits '' which must mean "no price", so
	// preprocess it to undefined rather than coercing to 0.
	price_minor: z.preprocess(
		(v) => (v === '' || v == null ? undefined : v),
		z.coerce.number().int().nonnegative().optional()
	),
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
		// The list description (#143) rides the same sanitize path as item notes: the
		// raw markdown is rendered to safe HTML here, and {@html} only ever touches this.
		descriptionHtml: renderNote(listRes.data.description),
		addForm: await superValidate(zod4(addItemSchema))
	};
};

export const actions: Actions = {
	add: async ({ request, locals, params }) => {
		const form = await superValidate(request, zod4(addItemSchema));
		if (!form.valid) return fail(400, { addForm: form });
		// Money requires a currency: if an amount is set, a 3-letter currency is too.
		const priceCurrency = (form.data.price_currency || '').trim().toUpperCase();
		if (form.data.price_minor != null && !priceCurrency) {
			return message(form, 'Add a 3-letter currency for the price.', { status: 400 });
		}
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
					form.data.price_minor != null && priceCurrency
						? { amount_minor: form.data.price_minor, currency: priceCurrency }
						: undefined,
				// priority isn't surfaced in the UI; send the default (the spec marks it
				// required, though the backend defaults it too).
				priority: 0
			}
		});
		if (err) return message(form, 'Could not add the item.', { status: 400 });
		// The client keeps its form on success (resetForm:false, so a ?/preview draft
		// survives), so a real add must clear it explicitly: return a fresh empty form
		// for the next item. invalidateAll (default) still refreshes the items list.
		return { addForm: await superValidate(zod4(addItemSchema)) };
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
		// Per-item thank-you override (#22/#111): "use list default" checkbox → null
		// (inherit); otherwise the textarea value ('' is an explicit per-item opt-out).
		const thankYouInherit = fd.get('thank_you_inherit') === 'on';
		const thank_you_template = thankYouInherit ? null : String(fd.get('thank_you_template') ?? '');
		if (!itemId || !name) return fail(400, { editError: 'Name is required.' });
		// Editable price (#128 price editing): an entered amount (major units) → Money.
		// Money requires a currency. Set-if-present, like the other edit fields.
		const priceAmountRaw = String(fd.get('price_amount') ?? '').trim();
		const priceCurrency = String(fd.get('price_currency') ?? '')
			.trim()
			.toUpperCase();
		let price: { amount_minor: number; currency: string } | undefined;
		if (priceAmountRaw !== '') {
			const amount = Number(priceAmountRaw);
			if (!Number.isFinite(amount) || amount < 0) {
				return fail(400, { editError: 'Price must be a non-negative number.' });
			}
			if (!priceCurrency) {
				return fail(400, { editError: 'Add a 3-letter currency for the price.' });
			}
			// 100-subunit assumption: major → minor. Technically wrong for zero-decimal
			// currencies (JPY); price is a hint and multi-currency (#24) is lowest priority.
			price = { amount_minor: Math.round(amount * 100), currency: priceCurrency };
		}
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
				allow_cobuy,
				thank_you_template,
				...(price ? { price } : {})
			}
		});
		if (err) return fail(400, { editError: 'Could not update the item.' });
		return { edited: true };
	},

	// List-level settings: co-buy default (#100) + thank-you note default (#22) +
	// reserver tier (#126).
	settings: async ({ request, locals, params }) => {
		const fd = await request.formData();
		const allow_cobuy = String(fd.get('allow_cobuy') ?? 'true') === 'true';
		const thank_you_template = String(fd.get('thank_you_template') ?? '');
		// List description (#143): raw markdown, stored as-is; the backend caps its
		// length and it is sanitized on render (renderNote), never here. "" clears it.
		const description = String(fd.get('description') ?? '');
		// Reserver tier (#126): an empty selection clears the override back to the
		// instance default (null, three-state per #111); otherwise it sets the tier.
		const reserverTierRaw = String(fd.get('reserver_tier') ?? '');
		const reserver_tier = reserverTierRaw === '' ? null : reserverTierRaw;
		const client = backendClient({ host: locals.host, token: locals.token });
		const { error: err } = await client.PATCH('/api/v1/lists/{listId}', {
			params: { path: { listId: params.id } },
			body: { allow_cobuy, thank_you_template, reserver_tier, description }
		});
		if (err) return fail(400, { settingsError: 'Could not update list settings.' });
		return { settingsSaved: true };
	},

	// Import items from an uploaded JSON/CSV file (#26). The file is read and
	// forwarded to the backend raw with the right Content-Type; the owner token
	// stays server-side. On a validation failure the backend returns per-row errors.
	import: async ({ request, locals, params }) => {
		const fd = await request.formData();
		const file = fd.get('file');
		if (!(file instanceof File) || file.size === 0) {
			return fail(400, { importError: 'Choose a JSON or CSV file to import.' });
		}
		const contentType = file.name.toLowerCase().endsWith('.csv') ? 'text/csv' : 'application/json';
		const res = await backendPostRaw({
			host: locals.host,
			token: locals.token,
			path: `/api/v1/lists/${params.id}/import`,
			contentType,
			body: await file.text()
		});
		if (res.status === 201) {
			const { created } = (await res.json()) as { created: number };
			return { imported: created };
		}
		// Surface the backend's per-row validation errors (if any), else a generic message.
		const problem = (await res.json().catch(() => null)) as {
			detail?: string;
			errors?: { row: number; message: string }[];
		} | null;
		return fail(res.status === 413 ? 413 : 400, {
			importError: problem?.detail ?? 'Could not import that file.',
			importRowErrors: problem?.errors ?? []
		});
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
