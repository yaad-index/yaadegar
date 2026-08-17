<script lang="ts">
	// Test harness: drives PageShell's empty-vs-content branch.
	import PageShell from './PageShell.svelte';

	let { isEmpty = false }: { isEmpty?: boolean } = $props();
</script>

{#snippet empty()}
	<p data-testid="empty">Nothing here yet</p>
{/snippet}

<PageShell {isEmpty} {empty}>
	<p data-testid="content">Real content</p>
</PageShell>
