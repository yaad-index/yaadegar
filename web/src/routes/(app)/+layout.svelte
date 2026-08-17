<script lang="ts">
	// The signed-in area shell (#205): the redesigned top bar + 800px content
	// column, owned here so the screens under it don't each fight over the layout.
	// PageShell provides the single <main>; the account actions (name, links, log
	// out) are this layout's own, passed through as the TopNav actions slot. No
	// screen content lives here. The empty-state slot is deliberately NOT used —
	// empty states belong to the screens, in place of their own content (#203).
	import type { Snippet } from 'svelte';
	import { resolve } from '$app/paths';
	import PageShell from '$lib/components/PageShell.svelte';
	import type { LayoutData } from './$types';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();
</script>

{#snippet accountActions()}
	<span class="text-ink">{data.user.name}</span>
	<a class="transition-colors hover:text-ink" href={resolve('/(app)/reservations')}>Reserved</a>
	{#if data.user.is_admin}
		<a class="transition-colors hover:text-ink" href={resolve('/admin')}>Admin</a>
	{/if}
	<a class="transition-colors hover:text-ink" href={resolve('/(app)/settings')}>Settings</a>
	<form method="post" action="/logout">
		<button class="transition-colors hover:text-ink" type="submit">Log out</button>
	</form>
{/snippet}

<PageShell brand="Yaadegar" home={resolve('/')} actions={accountActions}>
	{@render children()}
</PageShell>
