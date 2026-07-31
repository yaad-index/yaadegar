import { redirect, error } from '@sveltejs/kit';
import { backendClient } from '$lib/server/api';
import { clearAdminSession } from '$lib/server/adminsession';
import type { LayoutServerLoad } from './$types';

// Guards the whole /admin surface. The login page is the only route reachable
// without an admin session; every other /admin route requires one, verified
// against the backend's requireAdmin superadmin path (GET /admin/me). The owner
// token is never consulted here — only the separate admin token (ADR-0009 Cut 1b).
export const load: LayoutServerLoad = async ({ locals, cookies, url }) => {
	if (url.pathname === '/admin/login') return {};

	if (!locals.adminToken) redirect(303, '/admin/login');

	const client = backendClient({ host: locals.host, token: locals.adminToken });
	const { data, error: err, response } = await client.GET('/admin/me');
	if (err || !data) {
		// A gone/invalid admin session (or the surface disabled) → back to login.
		if (response.status === 401 || response.status === 403 || response.status === 404) {
			clearAdminSession(cookies);
			redirect(303, '/admin/login');
		}
		error(response.status || 500, 'Could not load the admin session.');
	}
	return { admin: data };
};
