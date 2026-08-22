<script lang="ts">
	// The gift-box glyph, shared (#207). It was inlined on the dashboard in three
	// places — the card icon chip, the empty state, and now the item-preview
	// placeholder — so it is extracted once here rather than copied a third time.
	// `currentColor` for the stroke, so the caller tints it (an accent or muted ink)
	// via a text-* utility; decorative by default (aria-hidden), since every caller
	// pairs it with real text.
	interface Props {
		/** Square size in px (width and height). */
		size?: number;
		/** Extra classes, e.g. a text-* colour utility. */
		class?: string;
	}
	let { size = 20, class: klass = '' }: Props = $props();
</script>

<svg
	width={size}
	height={size}
	viewBox="0 0 24 24"
	fill="none"
	stroke="currentColor"
	stroke-width="2"
	stroke-linecap="round"
	stroke-linejoin="round"
	aria-hidden="true"
	class={klass}
>
	<rect x="3" y="8" width="18" height="4" rx="1" />
	<path d="M12 8v13M5 12v7a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-7" />
	<path d="M12 8S9.5 3.5 7.5 4.5 7 8 12 8Zm0 0s2.5-4.5 4.5-3.5S17 8 12 8Z" />
</svg>
