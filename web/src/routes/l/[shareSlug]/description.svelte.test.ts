import { describe, it, expect, vi } from 'vitest';

// #143: the public giver load renders the list description through the SAME sanitize
// path as item notes (renderNote), exposing only descriptionHtml — {@html} never
// touches a raw description. The backend returns a hostile description; the load's
// descriptionHtml must be neutralized while keeping benign markdown.
const HOSTILE = '<img src=x onerror=alert(1)>\n\n[x](javascript:alert(1))\n\n*shown*';

vi.mock('$lib/server/api', () => ({
	backendClient: () => ({
		GET: async (path: string) => {
			if (path === '/public/{shareSlug}') {
				return {
					data: { title: 'T', description: HOSTILE, items: [] },
					error: undefined,
					response: { status: 200 }
				};
			}
			return { data: undefined, error: {}, response: { status: 404 } };
		}
	})
}));

import { load } from './+page.server';

const run = () =>
	(
		load as unknown as (e: {
			params: { shareSlug: string };
			locals: { host: string };
			cookies: { get: () => undefined; set: () => void; delete: () => void };
			url: URL;
		}) => Promise<{ descriptionHtml: string }>
	)({
		params: { shareSlug: 's1' },
		locals: { host: 't.example' },
		cookies: { get: () => undefined, set: () => {}, delete: () => {} },
		url: new URL('http://t.example/l/s1')
	});

describe('public giver-page description rendering (#143)', () => {
	it('exposes a sanitized descriptionHtml, keeping benign markdown', async () => {
		const { descriptionHtml } = await run();
		expect(descriptionHtml).toContain('<em>shown</em>');
		expect(descriptionHtml).not.toContain('onerror');
		expect(descriptionHtml).not.toContain('<img');
		expect(descriptionHtml.toLowerCase()).not.toContain('javascript:');
	});
});
