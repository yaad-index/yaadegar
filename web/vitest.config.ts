import { defineConfig } from 'vitest/config';

// A minimal Vitest config for plain-TypeScript unit tests (e.g. server utilities).
// It deliberately does NOT load the SvelteKit plugin — these tests exercise pure
// modules, so tests import by relative path rather than the `$lib` alias.
export default defineConfig({
	test: {
		include: ['src/**/*.{test,spec}.ts']
	}
});
