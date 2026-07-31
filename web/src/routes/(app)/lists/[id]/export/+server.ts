import { error } from '@sveltejs/kit';
import { backendGetRaw } from '$lib/server/api';
import type { RequestHandler } from './$types';

// Streams the owner's list export (#26) from the backend's raw export endpoint,
// attaching the owner token server-side so the JWT never reaches the browser. The
// backend sets the Content-Disposition; we forward it so the browser downloads.
export const GET: RequestHandler = async ({ params, url, locals }) => {
	const format = url.searchParams.get('format') === 'csv' ? 'csv' : 'json';
	const res = await backendGetRaw({
		host: locals.host,
		token: locals.token,
		path: `/api/v1/lists/${params.id}/export?format=${format}`
	});
	if (!res.ok) error(res.status === 404 ? 404 : 502, 'Could not export this list.');
	return new Response(res.body, {
		status: 200,
		headers: {
			'content-type': res.headers.get('content-type') ?? 'application/octet-stream',
			'content-disposition': res.headers.get('content-disposition') ?? 'attachment'
		}
	});
};
