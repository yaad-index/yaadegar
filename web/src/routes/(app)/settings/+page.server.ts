import { fail } from '@sveltejs/kit';
import { backendClient } from '$lib/server/api';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ locals }) => {
	const client = backendClient({ host: locals.host, token: locals.token });
	const [settingsRes, domainsRes] = await Promise.all([
		client.GET('/api/v1/settings'),
		client.GET('/api/v1/domains')
	]);
	return {
		settings: settingsRes.data ?? { oauth_google_enabled: false, google_client_configured: false },
		domains: domainsRes.data ?? []
	};
};

export const actions: Actions = {
	// Toggle Google login for the owner's own tenant. The backend writes only the
	// authenticated principal's tenant (from the session), so no tenant id is sent.
	toggle: async ({ request, locals }) => {
		const fd = await request.formData();
		const enabled = fd.get('oauth_google_enabled') === 'on';
		const client = backendClient({ host: locals.host, token: locals.token });
		const { data, error: err } = await client.PATCH('/api/v1/settings', {
			body: { oauth_google_enabled: enabled }
		});
		if (err || !data) return fail(400, { error: 'Could not update settings.' });
		return { settings: data, saved: true };
	},

	// Register a custom domain. The response carries the CNAME target and the TXT
	// verification token, which the reloaded list then shows as DNS instructions.
	addDomain: async ({ request, locals }) => {
		const fd = await request.formData();
		const hostname = String(fd.get('hostname') ?? '').trim();
		if (!hostname) return fail(400, { domainError: 'Enter a hostname.' });
		const client = backendClient({ host: locals.host, token: locals.token });
		const {
			data,
			error: err,
			response
		} = await client.POST('/api/v1/domains', {
			body: { hostname }
		});
		if (err || !data) {
			const msg =
				response.status === 409
					? 'That hostname is already registered.'
					: 'Could not add the domain.';
			return fail(response.status === 409 ? 409 : 400, { domainError: msg });
		}
		return { addedHostname: data.hostname };
	},

	// Re-check a domain's TXT record. An unverified result is a normal "not yet"
	// (DNS is still propagating), not an error — surface it as a retry hint.
	verifyDomain: async ({ request, locals }) => {
		const fd = await request.formData();
		const id = String(fd.get('id') ?? '');
		if (!id) return fail(400, { domainError: 'Missing domain.' });
		const client = backendClient({ host: locals.host, token: locals.token });
		const { data, error: err } = await client.POST('/api/v1/domains/{domainId}/verify', {
			params: { path: { domainId: id } }
		});
		if (err || !data) return fail(400, { domainError: 'Could not check verification.' });
		return { verifiedId: id, nowVerified: data.verified === true };
	},

	// Remove a custom domain.
	removeDomain: async ({ request, locals }) => {
		const fd = await request.formData();
		const id = String(fd.get('id') ?? '');
		if (!id) return fail(400, { domainError: 'Missing domain.' });
		const client = backendClient({ host: locals.host, token: locals.token });
		const { error: err } = await client.DELETE('/api/v1/domains/{domainId}', {
			params: { path: { domainId: id } }
		});
		if (err) return fail(400, { domainError: 'Could not remove the domain.' });
		return { removed: true };
	}
};
