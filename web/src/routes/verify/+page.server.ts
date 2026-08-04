import { fail, redirect } from '@sveltejs/kit';
import { backendClient } from '$lib/server/api';
import { setSession } from '$lib/server/session';
import { safeReturnTo } from '$lib/server/returnTo';
import type { Actions, PageServerLoad } from './$types';

// The verification token arrives in the emailed link's ?token=. We deliberately do
// NOT verify on load (GET): email link-scanners and browser prefetch would burn the
// single-use token before the owner clicks. Load only surfaces the token to a form
// that POSTs it (the same shape the password /reset page uses).
export const load: PageServerLoad = async ({ url }) => {
	return { token: url.searchParams.get('token') ?? '' };
};

export const actions: Actions = {
	default: async ({ request, cookies, locals, url }) => {
		const fd = await request.formData();
		const token = String(fd.get('token') ?? '');
		if (!token) return fail(400, { error: 'This verification link is missing its token.' });

		const client = backendClient({ host: locals.host });
		const { data, error: err } = await client.POST('/api/v1/auth/register/verify', {
			body: { token }
		});
		if (err || !data) {
			// Surface the backend's generic reason (invalid/expired/used token).
			return fail(400, {
				error: err?.detail ?? 'This verification link is invalid or has expired.'
			});
		}

		// Auto-login (ADR-0012): the verify response carries a fresh session, so set the
		// cookie and land the account in the app.
		setSession(cookies, data.access_token, data.expires_in, url.protocol === 'https:');
		// If registration started from a list that needs an account (#170), the register
		// step stashed a return path; consume it once and land back there. Validated to a
		// local path, so a tampered cookie can't become an open redirect.
		const returnTo = safeReturnTo(cookies.get('return_to'));
		cookies.delete('return_to', { path: '/' });
		redirect(303, returnTo ?? '/');
	}
};
