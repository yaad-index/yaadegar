import { error, fail } from '@sveltejs/kit';
import { superValidate, message } from 'sveltekit-superforms';
import { zod4 } from 'sveltekit-superforms/adapters';
import { z } from 'zod';
import { backendClient } from '$lib/server/api';
import { capsForList, addCap, removeCap } from '$lib/server/caps';
import type { Actions, PageServerLoad } from './$types';

// The giver may optionally leave a name and an email; both are used only
// server-side (e.g. decay reminders) and are never shown to anyone else. The item
// is chosen by the clicked Reserve button (name="item_id"). Quantity defaults to 1
// server-side, so the giver flow does not surface it.
const reserveSchema = z.object({
	item_id: z.string().min(1),
	giver_name: z.string().max(200).optional(),
	giver_email: z.string().email('Enter a valid email').optional().or(z.literal(''))
});

const isSecure = (url: URL) => url.protocol === 'https:';

export const load: PageServerLoad = async ({ params, locals, cookies }) => {
	const client = backendClient({ host: locals.host });
	const {
		data,
		error: err,
		response
	} = await client.GET('/public/{shareSlug}', {
		params: { path: { shareSlug: params.shareSlug } }
	});
	const form = await superValidate(zod4(reserveSchema));
	if (err || !data) {
		// A disabled list, or one past its event date, is a clean "closed" state — not
		// an error page (ADR-0002: the backend signals it with 410).
		if (response.status === 410) {
			return { closed: true as const, list: null, reservedItemIds: [] as string[], form };
		}
		error(response.status === 404 ? 404 : response.status || 502, 'This list is not available.');
	}
	return {
		closed: false as const,
		list: data,
		// Only the IDs of items THIS browser reserved — enough to drive the "you
		// reserved this" state and the release button. The capability tokens stay in
		// the httpOnly cookie and are read server-side in the release action; they are
		// deliberately NOT returned here, so they never reach client JS (ADR-0006 §4).
		reservedItemIds: Object.keys(capsForList(cookies, params.shareSlug)),
		form
	};
};

export const actions: Actions = {
	reserve: async ({ request, params, locals, cookies, url }) => {
		const form = await superValidate(request, zod4(reserveSchema));
		if (!form.valid) return fail(400, { form });

		const client = backendClient({ host: locals.host });
		const {
			data,
			error: err,
			response
		} = await client.POST('/public/{shareSlug}/items/{itemId}/reservations', {
			params: { path: { shareSlug: params.shareSlug, itemId: form.data.item_id } },
			body: {
				giver_name: form.data.giver_name || null,
				giver_email: form.data.giver_email || null,
				// The giver flow doesn't surface quantity; reserve a single unit. The spec
				// marks quantity required though it defaults to 1 server-side.
				quantity: 1
			}
		});
		if (err || !data?.reservation_id || !data?.capability_token) {
			if (response?.status === 409)
				return message(form, 'Someone just reserved this one.', { status: 409 });
			if (response?.status === 410)
				return message(form, 'This list is no longer active.', { status: 410 });
			return message(form, 'Could not reserve this item. Please try again.', { status: 400 });
		}
		// Persist the one-time token server-side; it never touches client JS.
		addCap(
			cookies,
			params.shareSlug,
			form.data.item_id,
			{ reservation_id: data.reservation_id, token: data.capability_token },
			isSecure(url)
		);
		return message(form, 'Reserved — thank you!');
	},

	release: async ({ request, params, locals, cookies, url }) => {
		const fd = await request.formData();
		const itemId = String(fd.get('item_id') ?? '');
		if (!itemId) return fail(400, { releaseError: 'Nothing to release.' });

		const entry = capsForList(cookies, params.shareSlug)[itemId];
		if (!entry) return fail(400, { releaseError: 'No reservation to release.' });

		const client = backendClient({ host: locals.host });
		const { error: err, response } = await client.DELETE('/public/reservations/{reservationId}', {
			params: { path: { reservationId: entry.reservation_id } },
			headers: { 'X-Capability-Token': entry.token }
		});
		// 204 = released. 401/404 means the reservation is already gone (released
		// elsewhere or auto-expired) — the stored capability is stale either way, so
		// drop it and show the item as available again.
		if (!err || response?.status === 401 || response?.status === 404) {
			removeCap(cookies, params.shareSlug, itemId, isSecure(url));
			return { released: true };
		}
		return fail(502, { releaseError: 'Could not release this reservation. Please try again.' });
	}
};
