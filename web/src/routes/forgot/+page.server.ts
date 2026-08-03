import { fail } from '@sveltejs/kit';
import { backendClient } from '$lib/server/api';
import type { Actions } from './$types';

export const actions: Actions = {
	// Start a password reset. The backend always answers 202 (enumeration-safe), and
	// we mirror that here: the UI shows the same neutral confirmation whether or not
	// the account exists, so it never reveals which.
	default: async ({ request, locals }) => {
		const fd = await request.formData();
		const identifier = String(fd.get('identifier') ?? '').trim();
		if (!identifier) return fail(400, { error: 'Enter your username or email.' });

		const client = backendClient({ host: locals.host });
		await client.POST('/api/v1/auth/password-reset/request', { body: { identifier } });
		return { sent: true };
	}
};
