<script lang="ts">
	// The list card's item-preview cluster (#207): up to three item thumbnails on
	// the right of the card, then a "+N" overflow chip for the rest. The tiles are
	// 28px circles that overlap into an avatar stack (measured from the design), each
	// later tile painting over the previous so the last is fully visible. An item
	// with no image of its own shows a gift glyph on the card's category-accent tint
	// — the accent governs the preview, not just the icon chip.
	//
	// Decorative by design: the cluster is aria-hidden. It previews the same items
	// the "N items" count chip already announces, and it shows objects, never who
	// reserved them — so it adds no information a screen reader needs and would
	// otherwise read as a run of anonymous images.
	import type { CategoryAccent } from '$lib/tokens';
	import GiftGlyph from './GiftGlyph.svelte';

	interface Preview {
		id?: string;
		image_url?: string | null;
	}

	interface Props {
		/** Up to three previews, in the list's item display order. */
		previews: Preview[];
		/** The list's full item count, for the overflow chip. */
		total: number;
		/** The card's accent — drives the imageless-item placeholder tint. */
		accent: CategoryAccent;
	}

	let { previews, total, accent }: Props = $props();

	// Items beyond the previewed few. previews is already capped at three by the API.
	const overflow = $derived(Math.max(0, total - previews.length));

	// 28px circle, ringed in the surface colour so overlapping tiles stay separated;
	// every tile past the first slides left to overlap the one before it.
	const tile = 'relative h-7 w-7 shrink-0 rounded-full ring-2 ring-surface';
</script>

{#if previews.length > 0}
	<div class="flex items-center" aria-hidden="true">
		{#each previews as preview, i (preview.id ?? i)}
			<div class={`${tile} overflow-hidden ${i > 0 ? '-ml-3' : ''}`}>
				{#if preview.image_url}
					<img src={preview.image_url} alt="" class="h-full w-full object-cover" />
				{:else}
					<div
						class={`flex h-full w-full items-center justify-center ${accent.placeholder} ${accent.icon}`}
					>
						<GiftGlyph size={14} />
					</div>
				{/if}
			</div>
		{/each}
		{#if overflow > 0}
			<div
				class={`${tile} -ml-3 flex items-center justify-center bg-surface-alt font-ui text-chip text-ink-muted`}
			>
				+{overflow}
			</div>
		{/if}
	</div>
{/if}
