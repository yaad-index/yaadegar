<script lang="ts">
	// Test harness: exercises Card's accent-chip branch and its link rendering.
	import Card from './Card.svelte';
	import { accentFor } from '$lib/tokens';

	let { withIcon = false }: { withIcon?: boolean } = $props();

	const accent = accentFor(0); // rose
</script>

{#snippet iconSnippet()}
	<span data-testid="card-icon">◆</span>
{/snippet}

<Card
	accent={withIcon ? accent : undefined}
	icon={withIcon ? iconSnippet : undefined}
	href="/lists/1"
>
	<span data-testid="card-body">Birthday list</span>
</Card>
