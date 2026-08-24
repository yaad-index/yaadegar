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
				// Svelte-transform + jsdom project: component tests, and server-logic tests
				// that transitively import Svelte deps (e.g. a route server module that
				// pulls in sveltekit-superforms). `$lib` is aliased for those server imports.
				plugins: [svelte(), svelteTesting()],
				resolve: {
					alias: {
						$lib: new URL('./src/lib', import.meta.url).pathname,
						// SvelteKit's $app/* virtual modules aren't provided outside the kit
						// plugin; stub them so server modules that transitively import them
						// (e.g. sveltekit-superforms) resolve. The stubs are never invoked.
						'$app/stores': new URL('./src/lib/testing/app-stubs/stores.ts', import.meta.url)
							.pathname,
						'$app/environment': new URL(
							'./src/lib/testing/app-stubs/environment.ts',
							import.meta.url
						).pathname,
						'$app/forms': new URL('./src/lib/testing/app-stubs/forms.ts', import.meta.url).pathname,
						'$app/navigation': new URL('./src/lib/testing/app-stubs/navigation.ts', import.meta.url)
							.pathname,
						'$app/paths': new URL('./src/lib/testing/app-stubs/paths.ts', import.meta.url).pathname,
						'$app/state': new URL('./src/lib/testing/app-stubs/state.ts', import.meta.url).pathname,
						// Read (not just resolved) by the root-page loader; the stub starts empty.
						'$env/dynamic/private': new URL(
							'./src/lib/testing/app-stubs/env-dynamic-private.ts',
							import.meta.url
						).pathname
					}
				},
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
