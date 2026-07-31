<script lang="ts">
	import type { Snippet } from 'svelte';
	import { resolve } from '$app/paths';
	import type { LayoutData } from './$types';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();
</script>

<div class="min-h-screen">
	<header class="border-b">
		<div class="mx-auto flex max-w-3xl items-center justify-between p-4">
			<a href={resolve('/')} class="font-bold">Yaadegar</a>
			<div class="flex items-center gap-3 text-sm">
				<span class="text-gray-600">{data.user.name}</span>
				<a href={resolve('/(app)/settings')} class="text-gray-600 underline">Settings</a>
				<form method="post" action="/logout">
					<button class="text-gray-600 underline">Log out</button>
				</form>
			</div>
		</div>
	</header>
	<main class="mx-auto max-w-3xl p-4">
		{@render children()}
	</main>
</div>
