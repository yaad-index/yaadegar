import { backendClient } from '$lib/server/api';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ locals }) => {
	const client = backendClient(locals);
	const { data } = await client.GET('/admin/tenants', { params: { query: { limit: 200 } } });
	return { tenants: data?.items ?? [] };
};
