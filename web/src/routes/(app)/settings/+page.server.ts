import { fail } from '@sveltejs/kit';
import { backendClient } from '$lib/server/api';
import { setSession } from '$lib/server/session';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ locals, url }) => {
	const client = backendClient(locals);
	const [settingsRes, domainsRes, ownerKeyRes, tokensRes] = await Promise.all([
		client.GET('/api/v1/settings'),
		client.GET('/api/v1/domains'),
		// The owner's shared-page key (#308). This read never mints one — null here
		// means the owner has not created a shared page, which is the ordinary state
		// and not an error.
		client.GET('/api/v1/me/owner-key'),
		// Personal access tokens (ADR-0016 §7), revoked and expired ones included.
		client.GET('/api/v1/me/tokens')
	]);
	return {
		settings: settingsRes.data ?? { oauth_google_enabled: false, google_client_configured: false },
		domains: domainsRes.data ?? [],
		ownerKey: ownerKeyRes.data?.owner_key ?? null,
		tokens: tokensRes.data ?? [],
		tokenExpiryDays: TOKEN_EXPIRY_DAYS,
		// Set when a password reset revoked tokens and sent the owner here to say so
		// (ADR-0016 §4). Only a whole positive number is shown.
		tokensRevoked: revokedCount(url.searchParams.get('tokens_revoked'))
	};
};

// The expiry choices offered when creating a token, in days. No expiry is offered
// too, but only as an explicit choice (ADR-0016 §1): nothing is preselected. The
// page renders its options from this list, via load, so the two cannot drift.
const TOKEN_EXPIRY_DAYS = [30, 90, 365];

function revokedCount(raw: string | null): number {
	const n = Number(raw);
	return Number.isInteger(n) && n > 0 ? n : 0;
}

export const actions: Actions = {
	// Create or rotate the owner's shared-page key (#308). Both are the same backend
	// operation — it replaces whatever is stored — so rotating is what revokes a link
	// that has been shared too widely. `rotating` rides along only so the page can
	// word its confirmation for what the owner actually did.
	ownerKey: async ({ request, locals }) => {
		const fd = await request.formData();
		const rotating = String(fd.get('rotating') ?? '') === 'true';
		const client = backendClient(locals);
		const { data, error: err } = await client.POST('/api/v1/me/owner-key', {});
		if (err || !data?.owner_key) {
			return fail(400, {
				ownerKeyError: rotating
					? 'Could not create a new link. Your existing one still works.'
					: 'Could not create your shared page.'
			});
		}
		return { ownerKey: data.owner_key, ownerKeyRotated: rotating, ownerKeyCreated: !rotating };
	},

	// Toggle Google login for the owner's own tenant. The backend writes only the
	// authenticated principal's tenant (from the session), so no tenant id is sent.
	toggle: async ({ request, locals }) => {
		const fd = await request.formData();
		const enabled = fd.get('oauth_google_enabled') === 'on';
		const client = backendClient(locals);
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
		const client = backendClient(locals);
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
		const client = backendClient(locals);
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
		// Personal access tokens survive a password change (ADR-0016 §4); the count
		// lets the page point at them. It is absent when the backend could not read it.
		return { passwordChanged: true, activeTokens: data.active_tokens ?? null };
	},

	// Create a personal access token (ADR-0016). Its value comes back once, in this
	// action's result, and is never stored by the app. The expiry is an explicit
	// choice: a number of days, or "never".
	createToken: async ({ request, locals }) => {
		const fd = await request.formData();
		const name = String(fd.get('name') ?? '').trim();
		const expiry = String(fd.get('expiry') ?? '');
		if (!name) return fail(400, { tokenError: 'Give the token a name.' });
		let expiryBody: { expires_at: string } | { never_expires: true };
		if (expiry === 'never') {
			expiryBody = { never_expires: true };
		} else {
			const days = Number(expiry);
			if (!TOKEN_EXPIRY_DAYS.includes(days)) {
				return fail(400, { tokenError: 'Choose when the token expires.' });
			}
			expiryBody = { expires_at: new Date(Date.now() + days * 24 * 60 * 60 * 1000).toISOString() };
		}
		const client = backendClient(locals);
		const {
			data,
			error: err,
			response
		} = await client.POST('/api/v1/me/tokens', { body: { name, ...expiryBody } });
		if (err || !data) {
			// 403 here is the recent-sign-in rule (§5): the page offers to sign in again.
			return fail(response.status || 400, {
				tokenError: err?.detail ?? 'Could not create the token.',
				tokenNeedsSignIn: response.status === 403
			});
		}
		return { createdToken: data.token, createdTokenName: data.access_token.name };
	},

	// Revoke a personal access token. It stops working on its next request.
	revokeToken: async ({ request, locals }) => {
		const fd = await request.formData();
		const id = String(fd.get('id') ?? '');
		if (!id) return fail(400, { tokenError: 'Missing token.' });
		const client = backendClient(locals);
		const { error: err } = await client.DELETE('/api/v1/me/tokens/{tokenId}', {
			params: { path: { tokenId: id } }
		});
		if (err) return fail(400, { tokenError: 'Could not revoke the token.' });
		return { tokenRevoked: true };
	},

	// Register a custom domain. The response carries the CNAME target and the TXT
	// verification token, which the reloaded list then shows as DNS instructions.
	addDomain: async ({ request, locals }) => {
		const fd = await request.formData();
		const hostname = String(fd.get('hostname') ?? '').trim();
		if (!hostname) return fail(400, { domainError: 'Enter a hostname.' });
		const client = backendClient(locals);
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
		const client = backendClient(locals);
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
		const client = backendClient(locals);
		const { error: err } = await client.DELETE('/api/v1/domains/{domainId}', {
			params: { path: { domainId: id } }
		});
		if (err) return fail(400, { domainError: 'Could not remove the domain.' });
		return { removed: true };
	}
};
