import { redirect } from '@sveltejs/kit';
import { clearAdminSession } from '$lib/server/adminsession';
import type { RequestHandler } from './$types';

export const POST: RequestHandler = ({ cookies }) => {
	clearAdminSession(cookies);
	redirect(303, '/admin/login');
};
