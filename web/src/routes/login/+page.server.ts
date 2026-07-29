import { fail, redirect } from '@sveltejs/kit';
import { superValidate, message } from 'sveltekit-superforms';
import { zod4 } from 'sveltekit-superforms/adapters';
import { z } from 'zod';
import { backendClient } from '$lib/server/api';
import { setSession } from '$lib/server/session';
import type { Actions, PageServerLoad } from './$types';

const loginSchema = z.object({
	username: z.string().min(1, 'Username is required'),
	password: z.string().min(1, 'Password is required')
});

export const load: PageServerLoad = async ({ locals }) => {
	// Already signed in → straight to the dashboard.
	if (locals.token) redirect(303, '/');
	return { form: await superValidate(zod4(loginSchema)) };
};

export const actions: Actions = {
	default: async ({ request, cookies, locals, url }) => {
		const form = await superValidate(request, zod4(loginSchema));
		if (!form.valid) return fail(400, { form });

		const client = backendClient({ host: locals.host });
		const { data, error, response } = await client.POST('/api/v1/auth/login', {
			body: { username: form.data.username, password: form.data.password }
		});
		if (error || !data) {
			if (response.status === 429) {
				return message(form, 'Too many attempts. Please try again in a little while.', {
					status: 429
				});
			}
			return message(form, 'Invalid username or password.', { status: 401 });
		}

		setSession(cookies, data.access_token, data.expires_in, url.protocol === 'https:');
		redirect(303, '/');
	}
};
