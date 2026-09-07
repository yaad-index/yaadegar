<script lang="ts">
	// The page shell (#199): the warm page background, the top navigation bar, and
	// the centred content column every screen sits inside. The empty state lives
	// here rather than being bolted on per screen — pass `isEmpty` with an `empty`
	// snippet and the shell centres it in the content area; otherwise it renders
	// `children`. TopNav props pass straight through.
	import type { Snippet } from 'svelte';
	import TopNav from './TopNav.svelte';

	interface Props {
		brand?: string;
		home?: string;
		actions?: Snippet;
		/** The designed empty state (typically an <EmptyState>) for this screen. */
		empty?: Snippet;
		/** When true and an `empty` snippet is given, show the empty state. */
		isEmpty?: boolean;
		children: Snippet;
	}

	let {
		brand = 'Yaadegar',
		home = '/',
		actions,
		empty,
		isEmpty = false,
		children
	}: Props = $props();
</script>

<div class="flex flex-1 flex-col bg-page">
	<TopNav {brand} {home} {actions} />
	<!-- w-full is load-bearing (#346). This <main> is a flex item in a column, and
	     auto inline margins beat `stretch` on the cross axis, so without an explicit
	     width its used width resolves to fit-content capped by max-width — making
	     the column as wide as whatever content is rendered. That turned a tab click
	     into a resize: 741px on List, 800px on Settings at a 1440px viewport. With
	     width:100% the used width is the container's, still capped by max-w-content
	     and still centred by the auto margins, but no longer a function of content.
	     `mx-auto max-w-*` alone is the correct centring idiom in a BLOCK container;
	     it is only wrong here because the parent is display:flex. -->
	<main class="mx-auto w-full max-w-content px-4 py-8">
		{#if isEmpty && empty}
			<div class="flex min-h-[50vh] items-center justify-center">
				{@render empty()}
			</div>
		{:else}
			{@render children()}
		{/if}
	</main>
</div>
