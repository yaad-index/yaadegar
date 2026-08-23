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
	/**
	 * Item-preview placeholder background (#207): the tile behind the gift glyph
	 * when a previewed item has no image. A deeper step of the accent than `chip`
	 * (the accent at 15% on white vs the chip's ~7%), so the same accent governs
	 * the preview cluster too, not just the icon chip.
	 */
	placeholder: string;
};

export const categoryAccents: readonly CategoryAccent[] = [
	{
		name: 'rose',
		icon: 'text-primary',
		chip: 'bg-primary-tint',
		placeholder: 'bg-primary-preview'
	},
	{ name: 'gold', icon: 'text-gold', chip: 'bg-gold-tint', placeholder: 'bg-gold-preview' },
	{ name: 'green', icon: 'text-green', chip: 'bg-green-tint', placeholder: 'bg-green-preview' }
] as const;

/**
 * Pick an accent for a card by a stable index. Wraps the set, so any integer
 * (including negatives, e.g. a running counter) maps to a valid accent.
 */
export function accentFor(index: number): CategoryAccent {
	const len = categoryAccents.length;
	return categoryAccents[((index % len) + len) % len];
}
