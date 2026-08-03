import { fail } from '@sveltejs/kit';
import { backendClient } from '$lib/server/api';
import type { Actions } from './$types';

export const actions: Actions = {
	// Start email self-registration (ADR-0012 cut 1a). The backend is enumeration-safe:
	// on any 2xx we show the same neutral "check your email" message whether or not the
	// email already exists, so the UI never reveals which. A 403 (registration disabled
	// on this instance) shows a distinct message; other failures surface the backend
	// reason. The captcha is a no-op for now, so we send an empty captcha_token.
	default: async ({ request, locals }) => {
		const fd = await request.formData();
		const email = String(fd.get('email') ?? '').trim();
		const password = String(fd.get('password') ?? '');
		const confirmPassword = String(fd.get('confirm_password') ?? '');
		if (!email) return fail(400, { error: 'Enter your email.' });
		if (!password) return fail(400, { error: 'Choose a password.' });
		if (password !== confirmPassword) {
			return fail(400, { error: 'The passwords do not match.' });
		}

		const client = backendClient({ host: locals.host });
		const { error: err, response } = await client.POST('/api/v1/auth/register', {
			body: { email, password, captcha_token: '' }
		});
		if (response.status === 403) {
			return fail(403, { disabled: true });
		}
		if (err) {
			// Surface the backend's real reason (e.g. a too-short password), falling back
			// to a generic line.
			return fail(response.status, {
				error: err.detail ?? 'Something went wrong. Please try again.'
			});
		}

		// Enumeration-neutral: the same confirmation whether the email was new or already
		// existed. No auto-login — the account is pending until it verifies its email.
		return { sent: true };
	}
};
