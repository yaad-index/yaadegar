import { fail, redirect } from '@sveltejs/kit';
import { backendClient } from '$lib/server/api';
import { setSession } from '$lib/server/session';
import type { Actions, PageServerLoad } from './$types';

// The reset token arrives in the emailed link's ?token=. We deliberately do NOT
// confirm on load (GET): email link-scanners and browser prefetch would burn the
// single-use token before the owner sets a password. Load only surfaces the token
// to a form that POSTs it (same shape the giver /confirm page uses).
export const load: PageServerLoad = async ({ url }) => {
	return { token: url.searchParams.get('token') ?? '' };
};

export const actions: Actions = {
	default: async ({ request, cookies, locals, url }) => {
		const fd = await request.formData();
		const token = String(fd.get('token') ?? '');
		const newPassword = String(fd.get('new_password') ?? '');
		const confirmPassword = String(fd.get('confirm_password') ?? '');
		if (!token) return fail(400, { error: 'This reset link is missing its token.' });
		if (!newPassword) return fail(400, { error: 'Enter a new password.' });
		if (newPassword !== confirmPassword) {
			return fail(400, { error: 'The passwords do not match.' });
		}

		const client = backendClient({ host: locals.host });
		const { data, error: err } = await client.POST('/api/v1/auth/password-reset/confirm', {
			body: { token, new_password: newPassword }
		});
		if (err || !data) {
			// Surface the backend's real reason (invalid/expired token, or a too-short
			// password), falling back to a generic line.
			return fail(400, { error: err?.detail ?? 'This reset link is invalid or has expired.' });
		}

		// Auto-login (ADR-0011): the confirm response carries a fresh session, so set
		// the cookie and land the owner in the app.
		setSession(cookies, data.access_token, data.expires_in, url.protocol === 'https:');
		redirect(303, '/');
	}
};
