import { fail } from '@sveltejs/kit';
import { backendClient } from '$lib/server/api';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ locals }) => {
	const client = backendClient({ host: locals.host, token: locals.token });
	const { data } = await client.GET('/api/v1/settings');
	return {
		settings: data ?? { oauth_google_enabled: false, google_client_configured: false }
	};
};

export const actions: Actions = {
	// Toggle Google login for the owner's own tenant. The backend writes only the
	// authenticated principal's tenant (from the session), so no tenant id is sent.
	default: async ({ request, locals }) => {
		const fd = await request.formData();
		// A checkbox sends its value only when checked, so presence == enabled.
		const enabled = fd.get('oauth_google_enabled') === 'on';
		const client = backendClient({ host: locals.host, token: locals.token });
		const { data, error: err } = await client.PATCH('/api/v1/settings', {
			body: { oauth_google_enabled: enabled }
		});
		if (err || !data) return fail(400, { error: 'Could not update settings.' });
		return { settings: data, saved: true };
	}
};
