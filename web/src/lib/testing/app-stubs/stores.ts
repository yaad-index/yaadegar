import { readable } from 'svelte/store';
// Test stub: sveltekit-superforms transitively imports $app/stores; these are never
// invoked by the server-logic tests, they only need to resolve.
export const page = readable({
	url: new URL('http://test.local/'),
	form: null,
	status: 200,
	data: {}
});
export const navigating = readable(null);
export const updated = Object.assign(readable(false), { check: async () => false });
export const getStores = () => ({ page, navigating, updated });
