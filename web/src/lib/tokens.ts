// The three category accents (#199) are a set, not per-screen decoration: each
// list card takes one accent for its icon chip. They are exposed as an ordered
// set so a screen can address them by index or name rather than hardcoding a
// colour per card — which is what keeps the set coherent as screens are added.

export type CategoryAccentName = 'rose' | 'gold' | 'green';

export type CategoryAccent = {
	name: CategoryAccentName;
	/** Foreground (icon) colour utility. */
	icon: string;
	/** Icon-chip background utility — the light tint paired with the tone. */
	chip: string;
};

export const categoryAccents: readonly CategoryAccent[] = [
	{ name: 'rose', icon: 'text-primary', chip: 'bg-primary-tint' },
	{ name: 'gold', icon: 'text-gold', chip: 'bg-gold-tint' },
	{ name: 'green', icon: 'text-green', chip: 'bg-green-tint' }
] as const;

/**
 * Pick an accent for a card by a stable index. Wraps the set, so any integer
 * (including negatives, e.g. a running counter) maps to a valid accent.
 */
export function accentFor(index: number): CategoryAccent {
	const len = categoryAccents.length;
	return categoryAccents[((index % len) + len) % len];
}
