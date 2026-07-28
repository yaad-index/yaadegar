<script lang="ts">
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';
	import { QueryClient, QueryClientProvider } from '@tanstack/svelte-query';

	// One TanStack Query client for the app (ADR-0006 §4). Feature routes use it
	// alongside SvelteKit load functions; F1 only proves it is wired.
	const queryClient = new QueryClient();

	let { children } = $props();
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
</svelte:head>

<QueryClientProvider client={queryClient}>
	{@render children()}
</QueryClientProvider>
