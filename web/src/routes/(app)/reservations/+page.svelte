<script lang="ts">
	import { enhance } from '$app/forms';
	import { resolve } from '$app/paths';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();
</script>

<svelte:head><title>Reserved by me · Yaadegar</title></svelte:head>

<h1 class="text-2xl font-bold">Reserved by me</h1>
<p class="mt-1 text-sm text-gray-600">
	Gifts you've reserved across lists. The list owner never learns who reserved.
</p>

{#if form?.releaseError}
	<p class="mt-3 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">{form.releaseError}</p>
{/if}

<ul class="mt-6 divide-y rounded border">
	{#each data.reservations as r (r.reservation_id)}
		<li class="flex items-center justify-between gap-3 p-3">
			<div class="min-w-0">
				<a
					href={resolve('/l/[shareSlug]', { shareSlug: r.share_slug })}
					class="font-medium hover:underline">{r.item_name}</a
				>
				<span class="ml-2 text-sm text-gray-500">on {r.list_title}</span>
				{#if r.quantity > 1}<span class="ml-2 text-xs text-gray-500">×{r.quantity}</span>{/if}
				{#if r.state === 'reserver_notified'}
					<span class="ml-2 rounded bg-amber-100 px-2 py-0.5 text-xs text-amber-800"
						>Reminder sent</span
					>
				{/if}
			</div>
			<form method="post" action="?/release" use:enhance>
				<input type="hidden" name="reservation_id" value={r.reservation_id} />
				<button class="rounded border px-2 py-1 text-xs hover:bg-gray-50">Release</button>
			</form>
		</li>
	{:else}
		<li class="p-3 text-gray-500">You haven't reserved anything yet.</li>
	{/each}
</ul>
