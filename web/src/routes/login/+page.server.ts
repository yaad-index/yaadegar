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

// oauthErrorMessages maps known provider/callback error codes to friendly copy;
// anything else falls back to a generic line. The raw code is never rendered as
// HTML (the page interpolates text, which Svelte escapes) — but mapping keeps the
// UI clean and avoids reflecting arbitrary provider strings verbatim.
const oauthErrorMessages: Record<string, string> = {
	access_denied: 'Sign-in was cancelled.',
	server_error: 'The sign-in provider had a problem. Please try again.',
	temporarily_unavailable: 'Sign-in is temporarily unavailable. Please try again.'
};

export const load: PageServerLoad = async ({ locals, url }) => {
	// Already signed in → straight to the dashboard.
	if (locals.token) redirect(303, '/');

	// Which login affordances to render for this host (ADR-0008 Cut 2). Defaults to
	// password-only if the backend can't be reached, so login still works.
	const client = backendClient({ host: locals.host });
	const { data: methods } = await client.GET('/api/v1/auth/methods');

	// Owner login is main-domain-only: on a custom domain both methods are false and
	// login_url points at the tenant's canonical login page — send the owner there.
	if (methods && !methods.password && !methods.google && methods.login_url) {
		redirect(303, methods.login_url);
	}

	const rawError = url.searchParams.get('oauth_error');
	const oauthError = rawError
		? (oauthErrorMessages[rawError] ?? 'Sign-in did not complete. Please try again.')
		: null;

	return {
		form: await superValidate(zod4(loginSchema)),
		methods: methods ?? { password: true, google: false, login_url: '' },
		host: locals.host,
		oauthError
	};
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
