<script lang="ts">
	// The top navigation bar (#199): the serif wordmark on the left, an actions
	// slot on the right, and the divider rule beneath. Layout only — the caller
	// supplies whatever nav items/links belong on a given screen through `actions`,
	// so this component makes no assumption about auth state.
	import type { Snippet } from 'svelte';

	interface Props {
		brand?: string;
		home?: string;
		actions?: Snippet;
	}

	let { brand = 'Yaadegar', home = '/', actions }: Props = $props();
</script>

<header class="border-b border-divider bg-surface">
	<div class="mx-auto flex max-w-content items-center justify-between px-4 py-3">
		<!-- eslint-disable svelte/no-navigation-without-resolve -- home is a caller-supplied
		     href (defaults to '/'); a shared nav cannot resolve the caller's route for them. -->
		<a href={home} class="font-display text-title font-semibold text-primary">{brand}</a>
		<!-- eslint-enable svelte/no-navigation-without-resolve -->
		{#if actions}
			<nav class="flex items-center gap-4 font-ui text-ui text-ink-muted">
				{@render actions()}
			</nav>
		{/if}
	</div>
</header>
