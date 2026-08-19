<script lang="ts">
	// The shared shell for the auth screens (#201): a centred ~400px panel on the
	// warm page background, the wordmark above a serif heading, the form, and an
	// optional link row beneath the panel. The white panel with the 1px border is
	// the auth-panel treatment (that border is what the `--border` token measures —
	// see #200); the wordmark and link row sit on the page background outside it.
	import type { Snippet } from 'svelte';

	interface Props {
		heading: string;
		/** Optional description line under the heading. */
		description?: Snippet;
		/** The form body. */
		children: Snippet;
		/** The link row rendered beneath the panel. */
		links?: Snippet;
	}

	let { heading, description, children, links }: Props = $props();
</script>

<!-- <main> is the page's landmark region: each auth page renders only this
     component, so this is the single main per page, matching PageShell. -->
<main class="flex min-h-screen items-center justify-center bg-page px-4 py-10">
	<div class="w-full max-w-[400px]">
		<div class="rounded-card border border-line bg-surface p-8">
			<div class="mb-6 flex flex-col items-center text-center">
				<!--
					Circular wordmark. PLACEHOLDER: the repo carries no Yaadegar brand
					mark (favicon.svg is the framework default), so this is a neutral
					rose monogram badge standing in for the design's mark — swap in the
					real asset the same way the typefaces get filled in (#199).
				-->
				<span
					class="mb-3 flex h-12 w-12 items-center justify-center rounded-full bg-primary font-display text-title text-white"
					aria-hidden="true">Y</span
				>
				<span class="font-display text-ui font-semibold tracking-[0.2em] text-primary"
					>YAADEGAR</span
				>
			</div>
			<h1 class="display-panel text-center font-display text-ink-heading">{heading}</h1>
			{#if description}
				<p class="mt-2 text-center font-ui text-body text-ink-muted">{@render description()}</p>
			{/if}
			<div class="mt-6">{@render children()}</div>
		</div>
		{#if links}
			<div class="mt-6 space-y-2 text-center font-ui text-ui text-ink-muted">{@render links()}</div>
		{/if}
	</div>
</main>
