import { error, fail } from '@sveltejs/kit';
import { backendClient } from '$lib/server/api';
import { renderNote } from '$lib/server/markdown';
import type { Actions, PageServerLoad } from './$types';

// The authenticated reserve-as-account view (#170): a signed-in giver reserves on a
// list by its share slug, bound to their account via POST /api/v1/me/reservations, so
// it lands in their dashboard. It works on any reserver tier (the tier is a floor) and
// is the only way to reserve a registered-tier list from the browser. Owner-anonymity
// is preserved — the public read strips every other giver's identity, and this view
// only ever surfaces the caller's own reservations.
export const load: PageServerLoad = async ({ params, locals }) => {
	// The public read renders the list + items; it needs no token and is already
	// owner-anonymous, so it is reused here rather than a second authenticated read.
	const pub = backendClient({ host: locals.host });
	const {
		data: list,
		error: err,
		response
	} = await pub.GET('/public/{shareSlug}', {
		params: { path: { shareSlug: params.shareSlug } }
	});
	if (err || !list) {
		// A disabled/expired list is a clean closed state (410), mirroring the public page.
		if (response.status === 410) {
			return {
				closed: true as const,
				list: null,
				reservedItems: {} as Record<string, ReservedEntry>,
				noteHtml: {} as Record<string, string>,
				descriptionHtml: ''
			};
		}
		error(response.status === 404 ? 404 : response.status || 502, 'This list is not available.');
	}

	// Which items this account already reserved, from its dashboard filtered to this
	// list — the reservation id drives the release action.
	const authed = backendClient({ host: locals.host, token: locals.token });
	const { data: mine } = await authed.GET('/api/v1/me/reservations', {
		params: { query: { limit: 200 } }
	});
	const reservedItems: Record<string, ReservedEntry> = {};
	for (const r of mine?.items ?? []) {
		if (r.share_slug === params.shareSlug) {
			reservedItems[r.item_id] = {
				reservation_id: r.reservation_id,
				state: r.state,
				quantity: r.quantity
			};
		}
	}

	return {
		closed: false as const,
		list,
		reservedItems,
		// Notes/description rendered to sanitized HTML server-side; {@html} only ever
		// touches these pre-sanitized maps (renderNote, ADR-0006 security boundary).
		noteHtml: Object.fromEntries((list.items ?? []).map((i) => [i.id ?? '', renderNote(i.note)])),
		descriptionHtml: renderNote(list.description)
	};
};

interface ReservedEntry {
	reservation_id: string;
	state: string;
	quantity: number;
}

export const actions: Actions = {
	// Reserve the clicked item as the session account. No capability token — the
	// reservation is account-bound and managed from the dashboard / this view.
	reserve: async ({ request, params, locals }) => {
		const fd = await request.formData();
		const itemId = String(fd.get('item_id') ?? '');
		if (!itemId) return fail(400, { reserveError: 'Nothing to reserve.' });

		const client = backendClient({ host: locals.host, token: locals.token });
		const {
			data,
			error: err,
			response
		} = await client.POST('/api/v1/me/reservations', {
			body: { share_slug: params.shareSlug, item_id: itemId, quantity: 1 }
		});
		if (err || !data?.reservation_id) {
			if (response?.status === 409)
				return fail(409, { reserveError: 'Someone just reserved this one.' });
			if (response?.status === 410)
				return fail(410, { reserveError: 'This list is no longer active.' });
			return fail(response?.status ?? 400, {
				reserveError: 'Could not reserve this item. Please try again.'
			});
		}
		return { reserved: true };
	},

	// Release one of the caller's own reservations on this list. Ownership is enforced
	// server-side (the backend 404s a reservation the account does not own).
	release: async ({ request, locals }) => {
		const fd = await request.formData();
		const reservationId = String(fd.get('reservation_id') ?? '');
		if (!reservationId) return fail(400, { releaseError: 'Nothing to release.' });

		const client = backendClient({ host: locals.host, token: locals.token });
		const { error: err, response } = await client.DELETE(
			'/api/v1/me/reservations/{reservationId}',
			{ params: { path: { reservationId } } }
		);
		// 204 = released. A 404 means it is already gone — treat as released so the item
		// reverts to available; any other failure is surfaced.
		if (err && response?.status !== 404) {
			return fail(response?.status ?? 400, {
				releaseError: 'Could not release this reservation. Please try again.'
			});
		}
		return { released: true };
	}
};
