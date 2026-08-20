import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

// The public marketing landing (#236). It is the site root, so it must render for a
// signed-OUT visitor rather than the old redirect-to-login. An authenticated visitor is
// sent straight on to their app — the dashboard at /lists, which itself re-routes a
// giver to their reserver view — so role-routing stays in one place and the marketing
// page only ever shows to signed-out visitors.
export const load: PageServerLoad = ({ locals }) => {
	if (locals.token) redirect(303, '/lists');
	return {};
};
