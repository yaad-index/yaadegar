<script lang="ts">
	// The empty state (#199) is a designed state, not a fallback: a centred icon,
	// a heading, a line of description and an optional action. It is a first-class
	// component so every screen's "nothing here yet" looks the same and carries a
	// real next step rather than a bare message.
	import type { Snippet } from 'svelte';

	interface Props {
		title: string;
		description?: string;
		/** Icon rendered in the chip above the title. */
		icon?: Snippet;
		/** Primary action (typically a Button) shown beneath the copy. */
		action?: Snippet;
	}

	let { title, description, icon, action }: Props = $props();
</script>

<div class="mx-auto max-w-sm text-center">
	{#if icon}
		<div
			class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-card bg-surface-accent text-ink-muted"
		>
			{@render icon()}
		</div>
	{/if}
	<h2 class="font-display text-title text-ink-heading">{title}</h2>
	{#if description}
		<p class="mt-2 font-ui text-body text-ink-muted">{description}</p>
	{/if}
	{#if action}
		<div class="mt-6 flex justify-center">{@render action()}</div>
	{/if}
</div>
