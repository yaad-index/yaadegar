import { describe, it, expect, vi } from 'vitest';

// #143: the owner list-detail load renders the list description through the SAME
// sanitize path as item notes (renderNote), exposing only descriptionHtml — {@html}
// never touches a raw description. Here the backend returns a hostile description and
// we assert the load's descriptionHtml is neutralized but keeps benign markdown.
const HOSTILE = '<script>alert(1)</script>\n\n[x](javascript:alert(1))\n\n**keep me**';

vi.mock('$lib/server/api', () => ({
	backendClient: () => ({
		GET: async (path: string) => {
			if (path === '/api/v1/lists/{listId}') {
				return {
					data: { id: 'l1', title: 'T', description: HOSTILE },
					error: undefined,
					response: { status: 200 }
				};
			}
			// items
			return { data: { items: [] }, error: undefined, response: { status: 200 } };
		}
	})
}));

import { load } from './+page.server';

const run = () =>
	(
		load as unknown as (e: {
			locals: { host: string; token: string };
			params: { id: string };
		}) => Promise<{ descriptionHtml: string }>
	)({ locals: { host: 't.example', token: 'tok' }, params: { id: 'l1' } });

describe('owner list-detail description rendering (#143)', () => {
	it('exposes a sanitized descriptionHtml, keeping benign markdown', async () => {
		const { descriptionHtml } = await run();
		expect(descriptionHtml).toContain('<strong>keep me</strong>');
		expect(descriptionHtml).not.toContain('<script');
		expect(descriptionHtml.toLowerCase()).not.toContain('javascript:');
	});
});
