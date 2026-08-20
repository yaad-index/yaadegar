import { redirect, error } from '@sveltejs/kit';
import { backendClient } from '$lib/server/api';
import { clearSession } from '$lib/server/session';
import type { LayoutServerLoad } from './$types';

// Guards the whole /admin surface. Admin is a capability on the owner account
// (ADR-0010): the surface is reached with the ordinary owner session, and access
// requires the is_admin flag on that account. No owner session → /login; a
// signed-in owner without the capability → back to the dashboard. Every /admin
// backend call is still authorized server-side by requireAdmin; this guard only
// governs what the browser is shown.
export const load: LayoutServerLoad = async ({ locals, cookies, url }) => {
	if (!locals.token) redirect(303, '/login');

	const client = backendClient({ host: locals.host, token: locals.token });
	const { data, error: err, response } = await client.GET('/api/v1/me');
	if (err || !data) {
		if (response.status === 401) {
			clearSession(cookies, url.protocol === 'https:');
			redirect(303, '/login');
		}
		error(response.status || 500, 'Could not load your account.');
	}
	if (!data.is_admin) redirect(303, '/');
	return { admin: data };
};
