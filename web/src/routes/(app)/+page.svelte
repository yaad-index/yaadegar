<script lang="ts">
	import { superForm } from 'sveltekit-superforms';
	import { resolve } from '$app/paths';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	// superForm captures the initial form once and owns its reactivity thereafter.
	// svelte-ignore state_referenced_locally
	const { form, errors, enhance, submitting } = superForm(data.form);
</script>

<svelte:head><title>Your lists · Yaadegar</title></svelte:head>

<h1 class="text-2xl font-bold">Your lists</h1>

<form method="post" action="?/create" use:enhance class="mt-4 flex gap-2">
	<input
		class="flex-1 rounded border p-2"
		name="title"
		placeholder="New list title"
		bind:value={$form.title}
	/>
	<button class="rounded bg-black px-3 py-2 text-white disabled:opacity-50" disabled={$submitting}>
		Create
	</button>
</form>
{#if $errors.title}<p class="mt-1 text-xs text-red-600">{$errors.title}</p>{/if}

<ul class="mt-6 divide-y rounded border">
	{#each data.lists as list (list.id)}
		<li>
			<a
				href={resolve('/(app)/lists/[id]', { id: list.id ?? '' })}
				class="flex items-center justify-between p-3 hover:bg-gray-50"
			>
				<span>{list.title}</span>
				<span class="text-sm text-gray-500">{list.item_count ?? 0} items</span>
			</a>
		</li>
	{:else}
		<li class="p-3 text-gray-500">No lists yet — create your first one above.</li>
	{/each}
</ul>
