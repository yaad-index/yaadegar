<script lang="ts">
	// The top navigation bar (#199): the serif wordmark on the left, an actions
	// slot on the right, and the divider rule beneath. Layout only — the caller
	// supplies whatever nav items/links belong on a given screen through `actions`,
	// so this component makes no assumption about auth state.
	//
	// Narrow-viewport behaviour (#211): the row WRAPS rather than clipping — with an
	// admin item present and a long account name the single nowrap row ran off a
	// phone and scrolled the page sideways. Wrapping is the interim mobile treatment
	// (the design set is desktop-only); the real mobile design is the designer's.
	import type { Snippet } from 'svelte';

	interface Props {
		brand?: string;
		home?: string;
		actions?: Snippet;
	}

	let { brand = 'Yaadegar', home = '/', actions }: Props = $props();
</script>

<header class="border-b border-divider bg-page">
	<div
		class="mx-auto flex max-w-content flex-wrap items-center justify-between gap-x-4 gap-y-2 px-4 py-3"
	>
		<!-- eslint-disable svelte/no-navigation-without-resolve -- home is a caller-supplied
		     href (defaults to '/'); a shared nav cannot resolve the caller's route for them. -->
		<a
			href={home}
			class="flex items-center gap-2.5 font-display text-title font-semibold text-primary"
		>
			<!-- Brand mark (#229): the rose rounded square with a white "Y", drawn in the
			     design on both surfaces. Responsive — 28px on mobile, 32px on desktop
			     (measured: solid-fill ~26 mobile / ~30 desktop). The wordmark's own
			     desktop-vs-mobile size is a separate, not-yet-pinned finding (#231). -->
			<span
				class="flex h-7 w-7 items-center justify-center rounded-lg bg-primary text-lg leading-none text-white sm:h-8 sm:w-8 sm:text-xl"
				aria-hidden="true">Y</span
			>
			{brand}
		</a>
		<!-- eslint-enable svelte/no-navigation-without-resolve -->
		{#if actions}
			<nav class="flex flex-wrap items-center gap-x-4 gap-y-1 font-ui text-ui text-ink-muted">
				{@render actions()}
			</nav>
		{/if}
	</div>
</header>
