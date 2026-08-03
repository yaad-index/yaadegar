import { fail, redirect } from '@sveltejs/kit';
import { superValidate, message } from 'sveltekit-superforms';
import { zod4 } from 'sveltekit-superforms/adapters';
import { z } from 'zod';
import { backendClient } from '$lib/server/api';
import type { Actions, PageServerLoad } from './$types';

const createListSchema = z.object({
	title: z.string().min(1, 'Title is required')
});

export const load: PageServerLoad = async ({ locals, parent }) => {
	// A giver owns no lists (the create box would 403), so their home is the reserver
	// dashboard, not this owner "your lists" page (ADR-0012 cut 3b, closing the cut-1a
	// cosmetic gap). Owners land here as before.
	const { user } = await parent();
	if (user?.role === 'giver') redirect(303, '/reservations');

	const client = backendClient({ host: locals.host, token: locals.token });
	const { data } = await client.GET('/api/v1/lists', { params: { query: { limit: 200 } } });
	return {
		lists: data?.items ?? [],
		form: await superValidate(zod4(createListSchema))
	};
};

export const actions: Actions = {
	create: async ({ request, locals }) => {
		const form = await superValidate(request, zod4(createListSchema));
		if (!form.valid) return fail(400, { form });

		const client = backendClient({ host: locals.host, token: locals.token });
		const { error: err } = await client.POST('/api/v1/lists', {
			body: { title: form.data.title }
		});
		if (err) return message(form, 'Could not create the list.', { status: 400 });
		return { form };
	}
};
