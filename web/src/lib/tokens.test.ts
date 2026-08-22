import { describe, it, expect } from 'vitest';
import { categoryAccents, accentFor } from './tokens';

// The accents are a set, and the point of the set is that a screen addresses it
// by index without knowing its size. These lock the ordering and the wrap.
describe('category accents', () => {
	it('is the rose / gold / green set, each with an icon and a chip utility', () => {
		expect(categoryAccents.map((a) => a.name)).toEqual(['rose', 'gold', 'green']);
		for (const accent of categoryAccents) {
			expect(accent.icon).toMatch(/^text-/);
			expect(accent.chip).toMatch(/^bg-/);
			expect(accent.placeholder).toMatch(/^bg-/);
		}
	});

	it('maps an index to an accent, wrapping the set', () => {
		expect(accentFor(0)).toBe(categoryAccents[0]);
		expect(accentFor(1)).toBe(categoryAccents[1]);
		expect(accentFor(2)).toBe(categoryAccents[2]);
		expect(accentFor(3)).toBe(categoryAccents[0]);
	});

	it('wraps negative indices too (a running counter never falls off the set)', () => {
		expect(accentFor(-1)).toBe(categoryAccents[2]);
		expect(accentFor(-3)).toBe(categoryAccents[0]);
	});
});
