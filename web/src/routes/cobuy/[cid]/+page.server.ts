import { fail } from '@sveltejs/kit';
import { backendClient } from '$lib/server/api';
import { findContribCap, removeContribCap } from '$lib/server/caps';
import { matchView } from '$lib/cobuy';
import type { Actions, PageServerLoad } from './$types';

const isSecure = (url: URL) => url.protocol === 'https:';

// The co-buying handshake. It is reached by contribution id (/cobuy/<id>) from the
// giver's "you're chipping in" panel; the capability token is read from the httpOnly
// yaadegar_cobuy_caps cookie server-side and never travels in the URL. v1 is
// same-browser only: the giver confirms from the device they pledged on. Load is
// read-only (there is no GET-match endpoint, so it shows the contribution's own
// status and never fetches — or leaks — any contact).
export const load: PageServerLoad = async ({ params, locals, cookies }) => {
	const c = params.cid;
	if (!c) return { state: 'invalid' as const };
	const cap = findContribCap(cookies, c);
	if (!cap) return { state: 'not_here' as const };

	const client = backendClient({ host: locals.host });
	const { data, response } = await client.GET('/public/contributions/{contributionId}', {
		params: { path: { contributionId: c } },
		headers: { 'X-Capability-Token': cap.token }
	});
	if (!data) {
		if (response?.status === 401 || response?.status === 404) return { state: 'invalid' as const };
		return { state: 'error' as const };
	}
	return {
		state: 'ok' as const,
		contributionId: c,
		status: data.status ?? 'pending',
		hasMatch: !!data.match_id
	};
};

export const actions: Actions = {
	decide: async ({ request, locals, cookies, url }) => {
		const fd = await request.formData();
		const contributionId = String(fd.get('contribution_id') ?? '');
		const decision = String(fd.get('decision') ?? '');
		if (decision !== 'confirm' && decision !== 'decline') {
			return fail(400, { decideError: 'Choose confirm or decline.' });
		}
		const cap = findContribCap(cookies, contributionId);
		if (!cap) {
			return fail(400, { decideError: 'This confirmation needs the device you pledged from.' });
		}

		const client = backendClient({ host: locals.host });
		// Re-read the contribution for its current match id, rather than trusting a
		// hidden field that may be stale.
		const { data: contrib } = await client.GET('/public/contributions/{contributionId}', {
			params: { path: { contributionId } },
			headers: { 'X-Capability-Token': cap.token }
		});
		const matchId = contrib?.match_id;
		if (!matchId) return fail(409, { decideError: 'There is no match to act on anymore.' });

		const { data: match, response } = await client.POST('/public/matches/{matchId}/confirm', {
			params: { path: { matchId } },
			headers: { 'X-Capability-Token': cap.token },
			body: { decision }
		});
		if (!match) {
			if (response?.status === 409)
				return fail(409, { decideError: 'This group buy has already been resolved.' });
			if (response?.status === 401) return fail(401, { decideError: 'Invalid capability token.' });
			return fail(502, { decideError: 'Could not record your decision. Please try again.' });
		}
		// matchView is the client-side anonymity choke point: contacts appear only when
		// the match is genuinely both_confirmed.
		const view = matchView(match);
		// A decline dissolves the match and releases every pledge — forget this
		// browser's capability so the item reverts to its normal offer state.
		if (view.released) removeContribCap(cookies, cap.shareSlug, cap.itemId, isSecure(url));
		return { decided: decision, view };
	}
};
