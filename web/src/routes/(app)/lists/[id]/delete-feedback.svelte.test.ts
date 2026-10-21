import { describe, it, expect, vi, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/svelte';
import { page } from '$app/state';
import Page from './+page.svelte';
import type { PageData } from './$types';

// #461: Delete said nothing in either direction. The action returned a deleteError
// that nothing rendered, so a refused delete looked exactly like a click that did
// not register, and a successful one left the owner to check the list themselves.

const backend = vi.hoisted(() => ({ fail: false, deleted: null as string | null }));
vi.mock('$lib/server/api', () => ({
	backendClient: () => ({
		DELETE: async (_path: string, opts: { params: { path: { itemId: string } } }) => {
			if (backend.fail) return { error: { detail: 'refused' } };
			backend.deleted = opts.params.path.itemId;
			return { error: undefined };
		}
	})
}));
vi.mock('$lib/server/markdown', () => ({ renderNote: (s: string) => s ?? '' }));

import { actions } from './+page.server';

const callDelete = (fields: Record<string, string>) => {
	const fd = new FormData();
	for (const [k, v] of Object.entries(fields)) fd.set(k, v);
	const request = new Request('http://t.example/lists/l1?/delete', { method: 'POST', body: fd });
	return (
		actions.delete as (e: {
			request: Request;
			locals: { host: string; token: string };
			params: { id: string };
		}) => Promise<unknown>
	)({ request, locals: { host: 't.example', token: 'tok' }, params: { id: 'l1' } });
};

const data = (items: unknown[]): PageData =>
	({
		list: {
			id: 'l1',
			title: 'Birthday',
			description: '',
			thank_you_template: '',
			allow_cobuy: true,
			reserver_tier: null,
			visibility: 'private',
			share_slug: 'abc123'
		},
		items,
		registrationEnabled: false,
		noteHtml: {},
		descriptionHtml: '',
		addForm: { id: 'add', valid: true, posted: false, errors: {}, data: {} }
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
	}) as any;

const remaining = {
	id: 'i1',
	name: 'Still wanted',
	quantity_wanted: 1,
	reserved_quantity: 0,
	availability: 'available',
	priority: 0
};

beforeEach(() => {
	backend.fail = false;
	backend.deleted = null;
	page.url = new URL('http://test.local/lists/l1') as typeof page.url;
});

describe('the delete action (#461)', () => {
	it('returns the deleted item’s name, because its row is gone when the result renders', async () => {
		const res = await callDelete({ item_id: 'i9', item_name: 'Cast iron pan' });
		expect(backend.deleted).toBe('i9');
		expect(res).toEqual({ deleted: true, deletedName: 'Cast iron pan' });
	});

	it('returns an error the page renders when the backend refuses', async () => {
		backend.fail = true;
		const res = (await callDelete({ item_id: 'i9', item_name: 'Cast iron pan' })) as {
			status: number;
			data: { deleteError?: string };
		};
		expect(res.status).toBe(400);
		expect(res.data.deleteError).toBe('Could not delete that item.');
	});
});

describe('delete feedback on the owner list page (#461)', () => {
	it('names the deleted item in a status region', () => {
		render(Page, {
			props: {
				data: data([remaining]),
				// eslint-disable-next-line @typescript-eslint/no-explicit-any
				form: { deleted: true, deletedName: 'Cast iron pan' } as any
			}
		});
		const status = screen.getAllByRole('status').find((el) => el.textContent?.includes('Deleted'));
		expect(status?.textContent).toContain('Deleted “Cast iron pan”.');
	});

	it('announces a refused delete as an alert rather than saying nothing', () => {
		render(Page, {
			props: {
				data: data([remaining]),
				// eslint-disable-next-line @typescript-eslint/no-explicit-any
				form: { deleteError: 'Could not delete that item.' } as any
			}
		});
		expect(screen.getByRole('alert')).toHaveTextContent('Could not delete that item.');
		// A refusal must not also claim success.
		expect(screen.getAllByRole('status').some((el) => el.textContent?.includes('Deleted'))).toBe(
			false
		);
	});

	it('posts the item name with every Delete button, live and archived', () => {
		const archived = {
			...remaining,
			id: 'i2',
			name: 'Finished with',
			archived_at: '2026-09-21T18:00:00Z'
		};
		const { container } = render(Page, {
			props: { data: data([remaining, archived]), form: null }
		});
		const forms = Array.from(container.querySelectorAll('form[action="?/delete"]'));
		expect(forms).toHaveLength(2);
		const names = forms.map(
			(f) => (f.querySelector('input[name="item_name"]') as HTMLInputElement | null)?.value
		);
		expect(names).toEqual(['Still wanted', 'Finished with']);
	});
});
