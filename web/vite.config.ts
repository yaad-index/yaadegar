import adapter from '@sveltejs/adapter-node';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [
		sveltekit({
			// Runes mode for project code (not libraries); removable in Svelte 6.
			compilerOptions: {
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},
			// Node adapter: containerized alongside the backend (ADR-0006 §5).
			adapter: adapter()
		})
	]
});
