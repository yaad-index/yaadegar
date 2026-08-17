<script lang="ts">
	// The list card (#199): a 104px-tall surface separated from the page by a soft
	// downward shadow (not a border — measured: the card and welcome banner carry
	// zero border pixels; the 1px --border outline belongs to the auth panel). An
	// optional category-accent icon chip sits on the left, content on the right.
	// When `href` is set the whole card is the link (the common list-item case);
	// otherwise it is a plain container. The accent comes from the addressable set
	// in $lib/tokens so cards across a screen stay coherent.
	import type { Snippet } from 'svelte';
	import type { CategoryAccent } from '$lib/tokens';

	interface Props {
		/** One accent from the category set; drives the icon-chip colours. */
		accent?: CategoryAccent;
		/** Icon rendered inside the accent chip (omit to render no chip). */
		icon?: Snippet;
		/** Render the card as a link to this href. */
		href?: string;
		children: Snippet;
	}

	let { accent, icon, href, children }: Props = $props();

	const container = 'flex min-h-card items-center gap-4 rounded-card bg-surface p-4 shadow-card';
</script>

{#snippet inner()}
	{#if icon}
		<span
			class={`flex h-11 w-11 shrink-0 items-center justify-center rounded-card ${accent?.chip ?? 'bg-surface-alt'} ${accent?.icon ?? 'text-ink-muted'}`}
		>
			{@render icon()}
		</span>
	{/if}
	<div class="min-w-0 flex-1">{@render children()}</div>
{/snippet}

{#if href}
	<!-- eslint-disable svelte/no-navigation-without-resolve -- href is a caller-supplied
	     value (a route this shared card cannot resolve on the caller's behalf). -->
	<a {href} class={`${container} transition-colors hover:bg-surface-alt`}>
		{@render inner()}
	</a>
	<!-- eslint-enable svelte/no-navigation-without-resolve -->
{:else}
	<div class={container}>
		{@render inner()}
	</div>
{/if}
