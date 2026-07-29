import { fail } from '@sveltejs/kit';
import { backendClient } from '$lib/server/api';
import { addConfirmedCap, confirmedCap, removeConfirmedCap } from '$lib/server/caps';
import type { Actions, PageServerLoad } from './$types';

const isSecure = (url: URL) => url.protocol === 'https:';

// The confirmation token arrives in the emailed link's ?token=. We deliberately do
// NOT confirm on load (GET): email link-scanners and browser prefetch would consume
// the one-time confirm and burn the capability-token issuance before the real giver
// clicks. Load only surfaces the token to a button that POSTs it (ADR-0007 §3).
export const load: PageServerLoad = async ({ url }) => {
	return { token: url.searchParams.get('token') ?? '' };
};

export const actions: Actions = {
	confirm: async ({ request, locals, cookies, url }) => {
		const fd = await request.formData();
		const token = String(fd.get('token') ?? '');
		if (!token) return fail(400, { state: 'invalid' as const });

		const client = backendClient({ host: locals.host });
		const {
			data,
			error: err,
			response
		} = await client.POST('/public/reservations/confirm', { body: { token } });

		if (err || !data?.reservation_id) {
			// 410 = the confirm window elapsed and the hold expired; a distinct, expected
			// state, not an error. 404 (unknown/already-spent token) or anything else is a
			// link that can't be used.
			if (response?.status === 410) return fail(410, { state: 'expired' as const });
			return fail(response?.status === 404 ? 404 : 502, { state: 'invalid' as const });
		}

		// First confirm returns the capability token; keep it server-side (never to
		// client JS, ADR-0006 §4) so this page can offer a release. An idempotent
		// re-confirm returns no token — the reservation is already active, just without
		// a fresh release handle in this browser.
		if (data.capability_token) {
			addConfirmedCap(
				cookies,
				{ reservation_id: data.reservation_id, token: data.capability_token },
				isSecure(url)
			);
			return { state: 'confirmed' as const, reservationId: data.reservation_id, canRelease: true };
		}
		return { state: 'confirmed' as const, reservationId: data.reservation_id, canRelease: false };
	},

	release: async ({ request, locals, cookies, url }) => {
		const fd = await request.formData();
		const reservationId = String(fd.get('reservation_id') ?? '');
		const entry = reservationId ? confirmedCap(cookies, reservationId) : undefined;
		if (!entry) return fail(400, { releaseError: 'No reservation to release.' });

		const client = backendClient({ host: locals.host });
		const { error: err, response } = await client.DELETE('/public/reservations/{reservationId}', {
			params: { path: { reservationId: entry.reservation_id } },
			headers: { 'X-Capability-Token': entry.token }
		});
		// 204 = released. 401/404 = the reservation is already gone (released elsewhere
		// or auto-expired); the stored capability is stale either way, so drop it.
		if (!err || response?.status === 401 || response?.status === 404) {
			removeConfirmedCap(cookies, reservationId, isSecure(url));
			return { released: true };
		}
		return fail(502, { releaseError: 'Could not release this reservation. Please try again.' });
	}
};
