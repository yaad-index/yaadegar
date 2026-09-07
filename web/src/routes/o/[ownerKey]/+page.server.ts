import { error } from '@sveltejs/kit';
import { backendClient } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// The owner page (#308): one durable link listing everything an owner has chosen to
// publish, instead of one link per list.
//
// There is no filtering to do here. The backend selects only listed lists when it
// reads them, so an unlisted list's share_slug never arrives — this load cannot leak
// one by rendering carelessly, because it never holds one.
export const load: PageServerLoad = async ({ params, locals }) => {
	const client = backendClient({ host: locals.host });
	const {
		data,
		error: err,
		response
	} = await client.GET('/public/owners/{ownerKey}', {
		params: { path: { ownerKey: params.ownerKey } }
	});

	if (err || !data) {
		error(response.status === 404 ? 404 : response.status || 502, 'This page is not available.');
	}

	// An owner who has published nothing yet is a real page with nothing on it, not a
	// missing one — the backend distinguishes those (200-empty vs 404) so an owner
	// opening their own link can tell "wrong key" from "nothing published".
	return {
		// null when the account has no display name of its own: the stored name falls
		// back to the account email, which must not become a public heading.
		displayName: data.display_name ?? null,
		lists: data.lists ?? []
	};
};
