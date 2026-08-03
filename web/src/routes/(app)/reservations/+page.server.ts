import { fail } from '@sveltejs/kit';
import { backendClient } from '$lib/server/api';
import type { Actions, PageServerLoad } from './$types';

// The reserver dashboard — "things I've reserved" across lists (ADR-0012 Decision 4 /
// #20). Any authenticated account (owner or giver) can see its own reservations; the
// backend keys the read on the session account and never discloses it to a list owner.
export const load: PageServerLoad = async ({ locals }) => {
	const client = backendClient({ host: locals.host, token: locals.token });
	const { data } = await client.GET('/api/v1/me/reservations', {
		params: { query: { limit: 200 } }
	});
	return { reservations: data?.items ?? [] };
};

export const actions: Actions = {
	// Release one of the caller's own reservations. Ownership is enforced server-side
	// (the backend 404s a reservation the account does not own).
	release: async ({ request, locals }) => {
		const fd = await request.formData();
		const id = String(fd.get('reservation_id') ?? '');
		if (!id) return fail(400, { releaseError: 'Missing reservation.' });
		const client = backendClient({ host: locals.host, token: locals.token });
		const { error: err } = await client.DELETE('/api/v1/me/reservations/{reservationId}', {
			params: { path: { reservationId: id } }
		});
		if (err) return fail(400, { releaseError: 'Could not release that reservation.' });
		return { released: true };
	}
};
