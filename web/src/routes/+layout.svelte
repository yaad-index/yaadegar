<script lang="ts">
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';
	import { QueryClient, QueryClientProvider } from '@tanstack/svelte-query';
	import { page } from '$app/state';
	import LegalFooter from '$lib/components/LegalFooter.svelte';

	// One TanStack Query client for the app (ADR-0006 §4). Feature routes use it
	// alongside SvelteKit load functions; F1 only proves it is wired.
	const queryClient = new QueryClient();

	let { children } = $props();

	// The legal footer belongs to the site, so it renders here rather than on each
	// page, and the landing page is the single exception: its own footer already
	// carries a Legal column (#301) and a second set of links would double up.
	//
	// Excluding one known route rather than listing the routes that opt in is
	// deliberate. An opt-in list is satisfied by every page that exists when it is
	// written and silently misses the next one added, which is exactly how these
	// links came to be unreachable on a login-rooted instance in the first place.
	const isLandingPage = $derived(page.route.id === '/');
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
</svelte:head>

<QueryClientProvider client={queryClient}>
	<!-- A column that fills the viewport, with the page above the footer. The page
	     shells grow into the space that is left (flex-1) rather than each demanding
	     its own 100vh: nested min-h-screen wrappers stack, which put the footer one
	     whole viewport below a short page and made links that are present in the
	     markup something no reader would ever scroll to. -->
	<div class="flex min-h-screen flex-col">
		{@render children()}
		{#if !isLandingPage}
			<LegalFooter />
		{/if}
	</div>
</QueryClientProvider>
