import { defineConfig } from 'vitest/config';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { svelteTesting } from '@testing-library/svelte/vite';

// Two test projects:
//  - unit:       plain-TypeScript unit tests (server utilities), node env, no Svelte
//                plugin — tests import by relative path rather than the `$lib` alias.
//  - components: Svelte component tests (*.svelte.test.ts) in a real DOM (jsdom), so
//                click → render behavior is actually exercised (#128 regression guard).
export default defineConfig({
	test: {
		projects: [
			{
				test: {
					name: 'unit',
					include: ['src/**/*.{test,spec}.ts'],
					exclude: ['src/**/*.svelte.{test,spec}.ts'],
					environment: 'node'
				}
			},
			{
				plugins: [svelte(), svelteTesting()],
				test: {
					name: 'components',
					include: ['src/**/*.svelte.{test,spec}.ts'],
					environment: 'jsdom',
					setupFiles: ['./vitest-setup-client.ts']
				}
			}
		]
	}
});
