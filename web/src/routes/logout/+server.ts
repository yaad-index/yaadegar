import { redirect } from '@sveltejs/kit';
import { clearSession } from '$lib/server/session';
import type { RequestHandler } from './$types';

export const POST: RequestHandler = ({ cookies, url }) => {
	clearSession(cookies, url.protocol === 'https:');
	redirect(303, '/login');
};
