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
	// image_url is prefilled by a URL preview and is also typed directly (#414). It
	// used to be preview-only, which made the scrape the single source for an item's
	// image — and the scrape fails often, so a failed one left an item that could not
	// be finished by hand. The scrape is a convenience over the manual path now, not
	// the only path.
	image_url: z.string().optional(),
	// What the LAST scrape put in image_url, carried on the form so the next scrape can
	// tell where the current value came from. If image_url still equals this, nobody
	// has touched it since; if it differs, a person typed it. That distinction is the
	// whole of the provenance rule below, and it needs no client-side bookkeeping.
	// Never sent to the backend — it exists only between the form and ?/preview.
	image_from_scrape: z.string().optional(),
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
	const [listRes, itemsRes, methodsRes] = await Promise.all([
		client.GET('/api/v1/lists/{listId}', { params: { path: { listId: params.id } } }),
		client.GET('/api/v1/lists/{listId}/items', {
			params: { path: { listId: params.id }, query: { limit: 200 } }
		}),
		// For the reserver-tier control: whether self-registration is enabled on this
		// instance, so the settings UI can warn when a `registered` tier is chosen on a
		// closed-registration instance (ADR-0012 Decision 5 / cut 3b).
		client.GET('/api/v1/auth/methods')
	]);
	if (listRes.error || !listRes.data) {
		error(listRes.response.status || 404, 'List not found.');
	}
	const items = itemsRes.data?.items ?? [];
	return {
		list: listRes.data,
		items,
		registrationEnabled: methodsRes.data?.registration_enabled ?? false,
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
			return message(form, "Couldn't fetch that page — fill the fields in by hand.");
		}
		if (data.name) form.data.name = data.name;
		if (data.url) form.data.url = data.url;
		// Provenance, not merely "keep what is there" (#414 review). A scrape that finds
		// no image must not undo work a PERSON did — but it must not preserve what a
		// PREVIOUS scrape left either, because that image was typed by nobody and
		// belongs to a URL no longer in the form. Keeping it produced an item with one
		// page's name and another page's picture, under a message saying there was no
		// image while the thumbnail showed one.
		//
		// The current value came from the last scrape exactly when it still equals what
		// that scrape set; anything else means a person edited it.
		const wasScraped =
			Boolean(form.data.image_url) && form.data.image_url === form.data.image_from_scrape;
		let keptTypedImage = false;
		if (data.image_url) {
			form.data.image_url = data.image_url;
			form.data.image_from_scrape = data.image_url;
		} else if (wasScraped) {
			form.data.image_url = undefined;
			form.data.image_from_scrape = undefined;
		} else {
			keptTypedImage = Boolean(form.data.image_url);
		}
		form.data.price_minor = data.price?.amount_minor ?? undefined;
		form.data.price_currency = data.price?.currency ?? undefined;
		// Name what came back (#414). An empty image box reads identically whether the
		// page had no image, the fetch was never run, or it failed — three states with
		// one appearance, and the owner cannot tell whether to retry, paste one, or
		// move on. Saying which fields were filled makes the empty ones legible.
		const filled = [
			data.name ? 'name' : '',
			data.price?.amount_minor != null ? 'price' : '',
			data.image_url ? 'image' : ''
		].filter(Boolean);
		if (filled.length === 0) {
			return message(form, 'Fetched that page, but found nothing to fill in — enter it by hand.');
		}
		if (!data.image_url) {
			// The message has to match what the box actually shows, or it asserts one
			// thing while the thumbnail displays another.
			return message(
				form,
				keptTypedImage
					? `Fetched ${filled.join(' and ')} — no image on that page, so the one you entered is unchanged.`
					: `Fetched ${filled.join(' and ')} — no image on that page, so paste an image link if you want one.`
			);
		}
		return message(form, `Fetched ${filled.join(', ')} — review and add.`);
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
		// Item image (#414). Sent on every edit that carries the field, so emptying the
		// box removes a wrong scraped image instead of leaving it stuck — a scrape
		// guesses, so "replace it" is not enough on its own.
		//
		// Absent and blank are kept apart for the same reason as a list title: the
		// form always carries this input, so a missing one means some other caller,
		// and clearing an image it never knew about would be a write nobody asked for.
		// It differs from `url` just above, which is set-if-present and so cannot be
		// cleared at all. That is the backend's set-if-present PATCH, unchanged here
		// rather than widened to a field nobody reported.
		const imageRaw = fd.get('image_url');
		const image_url = imageRaw === null ? undefined : String(imageRaw).trim();
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
				...(image_url !== undefined ? { image_url } : {}),
				note: note || undefined,
				allow_cobuy,
				thank_you_template,
				...(price ? { price } : {})
			}
		});
		if (err) return fail(400, { editError: 'Could not update the item.' });
		return { edited: true };
	},

	// List-level settings: title (#404) + co-buy default (#100) + thank-you note
	// default (#22) + reserver tier (#126).
	settings: async ({ request, locals, params }) => {
		const fd = await request.formData();
		// List title (#404, rule moved to the backend in #406). The storage write path
		// now trims and refuses a blank title and the API answers 400, so the check
		// below is the fast local message for the same rule rather than this surface's
		// own policy. The title is read differently from
		// the text fields below, and deliberately: `String(fd.get(x) ?? '')` collapses
		// an ABSENT field and a BLANK one into the same '', which is exactly how a
		// description is cleared. A title has no valid empty state (creation requires
		// one), so the two cases have to stay apart — blank is a user clearing the
		// list's name and is refused; absent means this form carried no title input,
		// and leaves the stored title untouched rather than failing.
		//
		// The absent branch is live, not defensive. This action is an HTTP endpoint and
		// the form is one caller of it, not the definition of it: a direct POST that
		// omits the field reaches here today — verified against a running instance, the
		// save succeeds and the stored title is left alone. The form always supplies the
		// input, which is the only reason the branch never fires from the UI.
		const titleRaw = fd.get('title');
		const title = titleRaw === null ? undefined : String(titleRaw).trim();
		if (title === '') {
			// Deliberately rejects the WHOLE save, not just the title: an owner who
			// blanks the title and edits the description in one submit loses the
			// description edit too. The alternative is a partial write where some
			// fields land and the title does not, leaving no way to tell from the form
			// which of them took. Refusing the submit keeps the settings atomic.
			return fail(400, { settingsError: 'Title is required.' });
		}
		const allow_cobuy = String(fd.get('allow_cobuy') ?? 'true') === 'true';
		const thank_you_template = String(fd.get('thank_you_template') ?? '');
		// List description (#143): raw markdown, stored as-is; the backend caps its
		// length and it is sanitized on render (renderNote), never here. "" clears it.
		const description = String(fd.get('description') ?? '');
		// Reserver tier (#126): an empty selection clears the override back to the
		// instance default (null, three-state per #111); otherwise it sets the tier.
		const reserverTierRaw = String(fd.get('reserver_tier') ?? '');
		const reserver_tier = reserverTierRaw === '' ? null : reserverTierRaw;
		// Whether this list appears on the owner's shared page (#308). The control has
		// two positions but the stored enum has three, so turning it off must not
		// silently rewrite an `unlisted` list into a `private` one: only a list that
		// was actually listed is changed, and the two non-listed values are left as
		// they are. Turning it off never affects the list's own share link.
		const current = String(fd.get('current_visibility') ?? 'private');
		const listed = String(fd.get('listed') ?? '') === 'on';
		const visibility = listed ? 'public' : current === 'public' ? 'private' : current;
		const client = backendClient({ host: locals.host, token: locals.token });
		const { error: err } = await client.PATCH('/api/v1/lists/{listId}', {
			params: { path: { listId: params.id } },
			body: {
				// Omitted when the form carried no title input at all: PATCH is merge-patch,
				// so an absent field leaves the stored title as it is.
				...(title !== undefined ? { title } : {}),
				allow_cobuy,
				thank_you_template,
				reserver_tier,
				description,
				visibility: visibility as 'public' | 'unlisted' | 'private'
			}
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
	},

	// Archive (#419). The item name is carried through the form rather than looked
	// up again, so the confirmation can say WHICH item was archived: the row it came
	// from has moved to the archived group by the time the result renders, and a
	// message that cannot name the item is one the owner has to go and verify.
	archive: async ({ request, locals }) => {
		const fd = await request.formData();
		const itemId = String(fd.get('item_id') ?? '');
		const itemName = String(fd.get('item_name') ?? '');
		if (!itemId) return fail(400, {});
		const client = backendClient({ host: locals.host, token: locals.token });
		const { data, error: err } = await client.POST('/api/v1/items/{itemId}/archive', {
			params: { path: { itemId } }
		});
		if (err) return fail(400, { archiveError: 'Could not archive that item.' });
		// Warnings are reported even though the archive succeeded — they are things
		// the owner could not see, not reasons it failed (#419 Q2: warn, not block).
		return {
			archived: true,
			archivedName: itemName,
			// flatMap rather than map+filter(Boolean): the code is optional in the
			// generated type and filter(Boolean) does not narrow it, so the page would
			// be handed (string | undefined)[] to index a copy map with.
			archiveWarnings: (data?.warnings ?? []).flatMap((w) => (w.code ? [w.code] : []))
		};
	},

	unarchive: async ({ request, locals }) => {
		const fd = await request.formData();
		const itemId = String(fd.get('item_id') ?? '');
		const itemName = String(fd.get('item_name') ?? '');
		if (!itemId) return fail(400, {});
		const client = backendClient({ host: locals.host, token: locals.token });
		const { error: err } = await client.DELETE('/api/v1/items/{itemId}/archive', {
			params: { path: { itemId } }
		});
		if (err) return fail(400, { archiveError: 'Could not put that item back.' });
		return { unarchived: true, archivedName: itemName };
	}
};
