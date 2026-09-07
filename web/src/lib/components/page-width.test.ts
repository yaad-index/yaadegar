import { describe, it, expect } from 'vitest';

// #346: every <main> in this app is a flex item in a column — the root layout is
// `flex min-h-screen flex-col` and PageShell has its own column. In flexbox, auto
// inline margins beat `stretch` on the cross axis, so `mx-auto max-w-*` WITHOUT an
// explicit width resolves to fit-content capped by max-width: the page column then
// takes its width from whatever content is rendered. That is what made a tab click
// resize the page by 59px, and made the owner page 200px narrower when empty than
// when populated.
//
// ⚠️ WHAT THIS TEST IS AND IS NOT. It asserts THE CLASS IS PRESENT. It does not and
// cannot assert the resulting layout — jsdom performs no layout, so no test in this
// suite can measure a width. The widths were verified in a real browser at a fixed
// viewport and those measurements live in the pull request, not here.
//
// It is still worth having, because the realistic regression is not someone deleting
// `w-full` from these files. It is a NEW route being added with the same idiom, which
// looks correct on its own and is why there were eight sites rather than one.
//
// Sources are read through Vite's glob rather than node:fs so the check needs no
// filesystem types in the app's tsconfig.
const sources = import.meta.glob('../../**/*.svelte', {
	query: '?raw',
	import: 'default',
	eager: true
}) as Record<string, string>;

/** Every <main …> opening tag that carries a class attribute, across the app. */
const mainTags = (): { file: string; cls: string }[] =>
	Object.entries(sources).flatMap(([file, src]) =>
		[...src.matchAll(/<main\b[^>]*class="([^"]*)"/g)].map((m) => ({ file, cls: m[1] }))
	);

describe('page column width does not follow content (#346)', () => {
	it('finds the <main> elements to check', () => {
		// Without this the assertion below passes vacuously if the glob resolves to
		// nothing — an empty set trivially satisfies "every element is fine".
		expect(mainTags().length).toBeGreaterThan(5);
	});

	it('every centred <main> also sets an explicit width', () => {
		const centred = mainTags().filter((m) => m.cls.includes('mx-auto') && m.cls.includes('max-w-'));
		expect(centred.length).toBeGreaterThan(0);

		const missing = centred.filter((m) => !/\bw-full\b/.test(m.cls));
		expect(
			missing.map((m) => `${m.file}: ${m.cls}`),
			'a <main> using mx-auto + max-w-* needs w-full, or its width follows its content (#346)'
		).toEqual([]);
	});
});
