import { fail } from '@sveltejs/kit';
import { backendClient } from '$lib/server/api';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ locals, params }) => {
	const client = backendClient({ host: locals.host, token: locals.token });
	const { data } = await client.GET('/admin/tenants/{tenantId}/users', {
		params: { path: { tenantId: params.tenantId } }
	});
	return { tenantId: params.tenantId, users: data?.items ?? [] };
};

// detailOf pulls the RFC 9457 `detail` string from a problem+json error body, for
// surfacing the 409 "owns N lists" message.
function detailOf(error: unknown): string | undefined {
	if (error && typeof error === 'object' && 'detail' in error) {
		const d = (error as { detail?: unknown }).detail;
		if (typeof d === 'string') return d;
	}
	return undefined;
}

export const actions: Actions = {
	// Create an owner or giver by email (no credential — set later via a login method).
	create: async ({ request, locals, params }) => {
		const fd = await request.formData();
		const email = String(fd.get('email') ?? '').trim();
		const role = String(fd.get('role') ?? 'owner') === 'giver' ? 'giver' : 'owner';
		if (!email) return fail(400, { actionError: 'Enter an email.' });
		const client = backendClient({ host: locals.host, token: locals.token });
		const { error, response } = await client.POST('/admin/tenants/{tenantId}/users', {
			params: { path: { tenantId: params.tenantId } },
			body: { email, role }
		});
		if (error) {
			return fail(response.status === 409 ? 409 : 400, {
				actionError:
					response.status === 409
						? 'A user with that email already exists.'
						: 'Could not create the user.'
			});
		}
		return { created: email };
	},

	// Change a user's role and/or ban flag. The change-role demotion 409 ("owns N
	// lists, reassign or delete first") is surfaced verbatim so the admin can act.
	update: async ({ request, locals, params }) => {
		const fd = await request.formData();
		const userId = String(fd.get('user_id') ?? '');
		if (!userId) return fail(400, { actionError: 'Missing user.' });
		const body: { role?: 'owner' | 'giver'; banned?: boolean } = {};
		if (fd.has('role')) body.role = String(fd.get('role')) === 'giver' ? 'giver' : 'owner';
		if (fd.has('banned')) body.banned = String(fd.get('banned')) === 'true';
		const client = backendClient({ host: locals.host, token: locals.token });
		const { error, response } = await client.PATCH('/admin/tenants/{tenantId}/users/{userId}', {
			params: { path: { tenantId: params.tenantId, userId } },
			body
		});
		if (error) {
			if (response.status === 409) {
				return fail(409, {
					actionError: detailOf(error) ?? 'Cannot demote: the account still owns lists.'
				});
			}
			return fail(400, { actionError: 'Could not update the user.' });
		}
		return { updated: userId };
	}
};
