import { describe, it, expect } from 'vitest';

// The legal footer is only useful if a reader reaches it (#302). It was present in
// the markup on every page and still unreachable: each page shell set its own
// `min-h-screen`, so nested inside the root layout's full-height column the shell
// demanded a viewport of its own and pushed the footer exactly one screen down.
// Measured in a browser, every page family overflowed by 69px — the footer's own
// height — so it was never in view on any of them.
//
// The invariant: exactly ONE element may claim the viewport's height, the root
// layout's column. Every shell inside it grows into what is left.
//
// This is a source-level assertion on purpose. jsdom performs no layout, so no test
// in this project can measure that the footer sits above the fold; asserting the
// links are in the document is satisfied by markup nobody can scroll to, which is
// the exact failure this replaces. What a test CAN own is the mechanism behind the
// measurement, and it fails the moment a new shell reintroduces min-h-screen.
//
// Files are read through import.meta.glob rather than node:fs, which keeps the test
// inside the toolchain's type environment (the project installs no node types).

const ROOT_LAYOUT = '/src/routes/+layout.svelte';

const sources = import.meta.glob('/src/**/*.svelte', {
	query: '?raw',
	import: 'default',
	eager: true
}) as Record<string, string>;

// Only class attributes count. Matching raw file text would let a passing mention in
// a comment — including the ones in this file's own subjects — stand in for the class
// actually being applied, which is the same "satisfied for the wrong reason" failure
// this file exists to replace.
function classAttributes(source: string): string[] {
	return [...source.matchAll(/class=(?:"([^"]*)"|'([^']*)')/g)].map((m) => m[1] ?? m[2] ?? '');
}

function hasClass(source: string, token: string): boolean {
	return classAttributes(source).some((attr) => attr.split(/\s+/).includes(token));
}

describe('viewport-height invariant', () => {
	it('finds the components to check', () => {
		// Guards the glob itself: an empty set would make the next test vacuously true,
		// which is precisely the failure mode this file is about.
		expect(Object.keys(sources).length).toBeGreaterThan(5);
		expect(sources).toHaveProperty(ROOT_LAYOUT);
	});

	it('allows exactly one full-viewport wrapper, in the root layout', () => {
		const offenders = Object.entries(sources)
			.filter(([path, source]) => path !== ROOT_LAYOUT && hasClass(source, 'min-h-screen'))
			.map(([path]) => path)
			.sort();

		expect(
			offenders,
			`These apply min-h-screen inside the root layout's full-height column, so each ` +
				`demands its own viewport and pushes the site footer a screen below the fold. ` +
				`Grow into the layout instead (flex-1).`
		).toEqual([]);
	});

	it('still has the one wrapper that makes the footer sit at the bottom', () => {
		// The other direction: removing it would let the footer ride up under short
		// content instead of resting at the bottom of the viewport.
		const root = sources[ROOT_LAYOUT];
		expect(hasClass(root, 'min-h-screen'), 'the root column must claim the viewport').toBe(true);
		expect(hasClass(root, 'flex-col'), 'the root column must stack page above footer').toBe(true);
	});
});
