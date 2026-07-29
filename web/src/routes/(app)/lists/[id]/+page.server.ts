import { error, fail } from '@sveltejs/kit';
import { superValidate, message } from 'sveltekit-superforms';
import { zod4 } from 'sveltekit-superforms/adapters';
import { z } from 'zod';
import { backendClient } from '$lib/server/api';
import type { Actions, PageServerLoad } from './$types';

const addItemSchema = z.object({
	name: z.string().min(1, 'Name is required'),
	url: z.string().url('Must be a valid URL').optional().or(z.literal('')),
	quantity_wanted: z.coerce.number().int().min(1).default(1)
});

export const load: PageServerLoad = async ({ locals, params }) => {
	const client = backendClient({ host: locals.host, token: locals.token });
	const [listRes, itemsRes] = await Promise.all([
		client.GET('/api/v1/lists/{listId}', { params: { path: { listId: params.id } } }),
		client.GET('/api/v1/lists/{listId}/items', {
			params: { path: { listId: params.id }, query: { limit: 200 } }
		})
	]);
	if (listRes.error || !listRes.data) {
		error(listRes.response.status || 404, 'List not found.');
	}
	return {
		list: listRes.data,
		items: itemsRes.data?.items ?? [],
		addForm: await superValidate(zod4(addItemSchema))
	};
};

export const actions: Actions = {
	add: async ({ request, locals, params }) => {
		const form = await superValidate(request, zod4(addItemSchema));
		if (!form.valid) return fail(400, { addForm: form });
		const client = backendClient({ host: locals.host, token: locals.token });
		const { error: err } = await client.POST('/api/v1/lists/{listId}/items', {
			params: { path: { listId: params.id } },
			body: {
				name: form.data.name,
				url: form.data.url || undefined,
				quantity_wanted: form.data.quantity_wanted,
				// priority isn't surfaced in the F2 UI; send the default (the spec marks
				// it required, though the backend defaults it too).
				priority: 0
			}
		});
		if (err) return message(form, 'Could not add the item.', { status: 400 });
		return { addForm: form };
	},

	edit: async ({ request, locals }) => {
		const fd = await request.formData();
		const itemId = String(fd.get('item_id') ?? '');
		const name = String(fd.get('name') ?? '').trim();
		const quantity = Number(fd.get('quantity_wanted') ?? 1);
		if (!itemId || !name) return fail(400, { editError: 'Name is required.' });
		const client = backendClient({ host: locals.host, token: locals.token });
		const { error: err } = await client.PATCH('/api/v1/items/{itemId}', {
			params: { path: { itemId } },
			body: { name, quantity_wanted: Number.isFinite(quantity) ? quantity : 1 }
		});
		if (err) return fail(400, { editError: 'Could not update the item.' });
		return { edited: true };
	},

	delete: async ({ request, locals }) => {
		const fd = await request.formData();
		const itemId = String(fd.get('item_id') ?? '');
		if (!itemId) return fail(400, {});
		const client = backendClient({ host: locals.host, token: locals.token });
		await client.DELETE('/api/v1/items/{itemId}', { params: { path: { itemId } } });
		return { deleted: true };
	}
};
