import { fail } from '@sveltejs/kit';
import { superValidate, message } from 'sveltekit-superforms';
import { zod4 } from 'sveltekit-superforms/adapters';
import { z } from 'zod';
import { backendClient } from '$lib/server/api';
import type { Actions, PageServerLoad } from './$types';

const createListSchema = z.object({
	title: z.string().min(1, 'Title is required')
});

export const load: PageServerLoad = async ({ locals }) => {
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
