import { fail } from '@sveltejs/kit';
import type { Cookies } from '@sveltejs/kit';
import { backendClient } from '$lib/server/api';
import { findContribCapByMatch, removeContribCap } from '$lib/server/caps';
import { matchView, matchLoadFailureState } from '$lib/cobuy';
import type { Actions, PageServerLoad } from './$types';

const isSecure = (url: URL) => url.protocol === 'https:';

// The co-buying handshake, reached two ways for the same match:
//   • cross-device from the emailed link  /cobuy/<matchId>?t=<scoped-token>
//   • same-browser from the "matched" panel /cobuy/<matchId> (cap token from cookie)
// The token authorizing GET-match / confirm is resolved server-side; the cap token
// never leaves the httpOnly cookie. The scoped token is already public in the URL,
// so echoing it into the form is no extra exposure.
function resolveToken(url: URL, cookies: Cookies, matchId: string) {
	const scoped = url.searchParams.get('t');
	if (scoped) return { token: scoped, scoped: true };
	const cap = findContribCapByMatch(cookies, matchId);
	return { token: cap?.token, scoped: false };
}

export const load: PageServerLoad = async ({ params, url, locals, cookies }) => {
	const t = url.searchParams.get('t') ?? '';
	const { token, scoped } = resolveToken(url, cookies, params.matchId);
	if (!token) return { state: 'invalid' as const, t };

	const client = backendClient({ host: locals.host });
	const { data, response } = await client.GET('/public/matches/{matchId}', {
		params: { path: { matchId: params.matchId } },
		headers: { 'X-Capability-Token': token }
	});
	if (!data) {
		return { state: matchLoadFailureState(response?.status, scoped), t };
	}
	return { state: 'ok' as const, view: matchView(data), t };
};

export const actions: Actions = {
	decide: async ({ request, params, locals, cookies, url }) => {
		const fd = await request.formData();
		const decision = String(fd.get('decision') ?? '');
		const scoped = String(fd.get('t') ?? '');
		if (decision !== 'confirm' && decision !== 'decline') {
			return fail(400, { decideError: 'Choose confirm or decline.' });
		}
		const capEntry = scoped ? undefined : findContribCapByMatch(cookies, params.matchId);
		const token = scoped || capEntry?.token;
		if (!token) return fail(400, { decideError: 'This confirmation link is no longer valid.' });

		const client = backendClient({ host: locals.host });
		const { data: match, response } = await client.POST('/public/matches/{matchId}/confirm', {
			params: { path: { matchId: params.matchId } },
			headers: { 'X-Capability-Token': token },
			body: { decision }
		});
		if (!match) {
			if (response?.status === 409)
				return fail(409, { decideError: 'This group buy has already been resolved.' });
			if (response?.status === 410)
				return fail(410, { decideError: 'This confirmation link has expired.' });
			if (response?.status === 401)
				return fail(401, { decideError: 'This confirmation link is no longer valid.' });
			return fail(502, { decideError: 'Could not record your decision. Please try again.' });
		}
		const view = matchView(match);
		// On a decline, drop the same-browser capability so the item reverts cleanly.
		if (view.released && capEntry) {
			removeContribCap(cookies, capEntry.shareSlug, capEntry.itemId, isSecure(url));
		}
		return { decided: decision, view };
	}
};
