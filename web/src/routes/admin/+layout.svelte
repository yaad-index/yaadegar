<script lang="ts">
	import type { Snippet } from 'svelte';
	import { resolve } from '$app/paths';
	import type { LayoutData } from './$types';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();
</script>

<div class="min-h-screen">
	<!-- The admin chrome renders only for an authenticated admin (not on the login
	     page). This is a distinct surface: no links to owner or public areas. -->
	{#if data.admin}
		<header class="border-b bg-gray-900 text-white">
			<div class="mx-auto flex max-w-3xl items-center justify-between p-4">
				<a href={resolve('/admin')} class="font-bold">Yaadegar admin</a>
				<div class="flex items-center gap-3 text-sm">
					<span class="text-gray-300">{data.admin.username}</span>
					<form method="post" action="/admin/logout">
						<button class="text-gray-300 underline">Sign out</button>
					</form>
				</div>
			</div>
		</header>
	{/if}
	<main class="mx-auto max-w-3xl p-4">
		{@render children()}
	</main>
</div>
