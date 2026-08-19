<script lang="ts">
	// The signed-in area shell (#205): the redesigned top bar + 800px content
	// column, owned here so the screens under it don't each fight over the layout.
	// PageShell provides the single <main>; the account actions are this layout's
	// own (AccountNav), given the current path so it can mark the active route.
	// The empty-state slot is deliberately NOT used — empty states belong to the
	// screens, in place of their own content (#203).
	import type { Snippet } from 'svelte';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import PageShell from '$lib/components/PageShell.svelte';
	import AccountNav from '$lib/components/AccountNav.svelte';
	import MobileNav from '$lib/components/MobileNav.svelte';
	import type { LayoutData } from './$types';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();
</script>

{#snippet accountActions()}
	<!-- Desktop inline nav (hidden < sm) + the mobile hamburger/drawer (hidden >= sm);
	     the two are mutually exclusive so exactly one shows per viewport (#229). -->
	<AccountNav
		name={data.user.name ?? ''}
		isAdmin={data.user.is_admin ?? false}
		pathname={page.url.pathname}
	/>
	<MobileNav
		name={data.user.name ?? ''}
		isAdmin={data.user.is_admin ?? false}
		pathname={page.url.pathname}
	/>
{/snippet}

<PageShell brand="Yaadegar" home={resolve('/')} actions={accountActions}>
	{@render children()}
</PageShell>
