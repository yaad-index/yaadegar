import { fail } from '@sveltejs/kit';
import { backendClient } from '$lib/server/api';
import { setSession } from '$lib/server/session';
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

	// Update the signed-in account's own display name (#185). A blank name clears the
	// custom value; the backend falls it back to the account email. On success the
	// default enhance invalidation reloads /api/v1/me, so the header and the prefilled
	// field reflect the new name immediately.
	updateName: async ({ request, locals }) => {
		const fd = await request.formData();
		const name = String(fd.get('name') ?? '');
		const client = backendClient({ host: locals.host, token: locals.token });
		const { error: err, response } = await client.PUT('/api/v1/me/profile', {
			body: { name }
		});
		if (err) {
			return fail(response.status || 400, {
				nameError: err?.detail ?? 'Could not save your name.'
			});
		}
		return { nameSaved: true };
	},

	// Change the owner's own password (ADR-0011 cut 2). On success the backend
	// re-issues THIS session (a fresh token at the bumped credential version), so we
	// swap the session cookie to keep the owner logged in here while every other
	// session drops. A wrong current password / policy failure surfaces the backend's
	// real reason (per #144), not a generic error.
	changePassword: async ({ request, cookies, locals, url }) => {
		const fd = await request.formData();
		const currentPassword = String(fd.get('current_password') ?? '');
		const newPassword = String(fd.get('new_password') ?? '');
		const confirmPassword = String(fd.get('confirm_password') ?? '');
		if (!currentPassword || !newPassword) {
			return fail(400, { passwordError: 'Enter your current and new password.' });
		}
		if (newPassword !== confirmPassword) {
			return fail(400, { passwordError: 'The new passwords do not match.' });
		}
		const client = backendClient({ host: locals.host, token: locals.token });
		const {
			data,
			error: err,
			response
		} = await client.PUT('/api/v1/me/password', {
			body: { current_password: currentPassword, new_password: newPassword }
		});
		if (err || !data) {
			return fail(response.status || 400, {
				passwordError: err?.detail ?? 'Could not change your password.'
			});
		}
		// The change invalidated every session, including this cookie's old token —
		// install the re-issued one so the owner stays signed in on this device.
		setSession(cookies, data.access_token, data.expires_in, url.protocol === 'https:');
		return { passwordChanged: true };
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
