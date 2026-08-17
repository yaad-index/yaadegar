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

<div class="min-h-screen bg-page">
	<TopNav {brand} {home} {actions} />
	<main class="mx-auto max-w-content px-4 py-8">
		{#if isEmpty && empty}
			<div class="flex min-h-[50vh] items-center justify-center">
				{@render empty()}
			</div>
		{:else}
			{@render children()}
		{/if}
	</main>
</div>
