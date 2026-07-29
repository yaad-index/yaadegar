import { error, redirect } from '@sveltejs/kit';
import { backendClient } from '$lib/server/api';
import { clearSession } from '$lib/server/session';
import type { LayoutServerLoad } from './$types';

export const load: LayoutServerLoad = async ({ locals, cookies }) => {
	if (!locals.token) redirect(303, '/login');

	// The backend is the source of truth for the session; a 401 means the token is
	// gone/expired, so clear the cookie and send the owner back to log in.
	const client = backendClient({ host: locals.host, token: locals.token });
	const { data, error: err, response } = await client.GET('/api/v1/me');
	if (err || !data) {
		if (response.status === 401) {
			clearSession(cookies);
			redirect(303, '/login');
		}
		error(response.status || 500, 'Could not load your account.');
	}
	return { user: data };
};
