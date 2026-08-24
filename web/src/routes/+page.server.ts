import { redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';
import { resolveRootPage } from '$lib/server/rootPage';

// The site root (#236, #256). An authenticated visitor is always sent on to their app
// — the dashboard at /lists, which re-routes a giver onward — so role-routing stays in
// one place and this loader only ever decides what a SIGNED-OUT visitor sees. That
// decision is the operator's, via YAADEGAR_ROOT_PAGE (ADR-0015): the bundled marketing
// landing (default, unchanged), a redirect to /login, or the bundled layout with the
// operator's own words. Unset behaves exactly as today.
export const load: PageServerLoad = ({ locals }) => {
	if (locals.token) redirect(303, '/lists');

	const root = resolveRootPage(env);
	if (root.mode === 'login') redirect(303, '/login');
	// bundled → undefined (the page renders its shipped copy); custom → the strings.
	return { custom: root.mode === 'custom' ? root.strings : undefined };
};
