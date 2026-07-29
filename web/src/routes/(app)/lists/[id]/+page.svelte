<script lang="ts">
	import { superForm } from 'sveltekit-superforms';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	// superForm captures the initial form once and owns its reactivity thereafter.
	// svelte-ignore state_referenced_locally
	const { form, errors, message, enhance, submitting } = superForm(data.addForm);

	// The public giver link is this list's share slug on the current (tenant) origin.
	// share_slug already rides in the load data — the owner just needs it surfaced to
	// send to givers (the giver flow lives at /l/<slug>).
	const shareUrl = $derived(`${page.url.origin}/l/${data.list.share_slug ?? ''}`);
	let copied = $state<'idle' | 'ok' | 'fail'>('idle');
	let resetTimer: ReturnType<typeof setTimeout> | undefined;
	async function copyShare() {
		clearTimeout(resetTimer);
		try {
			// clipboard.writeText needs a secure context (https or localhost); it throws
			// otherwise, so fall back to the always-selectable input below.
			await navigator.clipboard.writeText(shareUrl);
			copied = 'ok';
		} catch {
			copied = 'fail';
		}
		// Let the confirmation/nudge fade back to idle instead of lingering.
		resetTimer = setTimeout(() => (copied = 'idle'), 2500);
	}

	// Availability is derived by the backend; it never carries reserver identity
	// (ADR-0002 §5). We show only the state and the reserved count.
	const availabilityLabel: Record<string, string> = {
		available: 'Available',
		reserved: 'Reserved',
		co_buying: 'Co-buying',
		purchased: 'Purchased'
	};
</script>

<svelte:head><title>{data.list.title} · Yaadegar</title></svelte:head>

<a href={resolve('/')} class="text-sm text-gray-500 hover:underline">← Your lists</a>
<h1 class="mt-1 text-2xl font-bold">{data.list.title}</h1>

<!-- Share link. The selectable input is always the fallback (works with no JS and in
     non-secure contexts); the copy button is a progressive enhancement on top. -->
<section class="mt-3 rounded border bg-gray-50 p-3">
	<p class="text-sm font-medium">Share this list</p>
	<p class="mt-0.5 text-xs text-gray-600">
		Send this link to givers — they can reserve without an account, and you never see who reserved
		what.
	</p>
	<div class="mt-2 flex gap-2">
		<input
			class="flex-1 rounded border p-2 text-sm"
			readonly
			value={shareUrl}
			aria-label="Public share link"
			onclick={(e) => e.currentTarget.select()}
		/>
		<button type="button" class="rounded bg-black px-3 py-2 text-sm text-white" onclick={copyShare}>
			Copy
		</button>
	</div>
	{#if copied === 'ok'}
		<p class="mt-1 text-xs text-green-700" role="status">Link copied.</p>
	{:else if copied === 'fail'}
		<p class="mt-1 text-xs text-gray-600" role="status">
			Couldn't copy automatically — select the link above and copy it.
		</p>
	{/if}
</section>

<!-- Add item -->
<form method="post" action="?/add" use:enhance class="mt-4 space-y-2 rounded border p-3">
	<div class="flex gap-2">
		<input
			class="flex-1 rounded border p-2"
			name="name"
			placeholder="Item name"
			bind:value={$form.name}
		/>
		<input
			class="w-20 rounded border p-2"
			name="quantity_wanted"
			type="number"
			min="1"
			bind:value={$form.quantity_wanted}
		/>
	</div>
	<input
		class="w-full rounded border p-2"
		name="url"
		placeholder="Link (optional)"
		bind:value={$form.url}
	/>
	<div class="flex items-center gap-3">
		<button
			class="rounded bg-black px-3 py-2 text-white disabled:opacity-50"
			disabled={$submitting}
		>
			Add item
		</button>
		{#if $errors.name}<span class="text-xs text-red-600">{$errors.name}</span>{/if}
		{#if $errors.url}<span class="text-xs text-red-600">{$errors.url}</span>{/if}
		{#if $message}<span class="text-xs text-red-600">{$message}</span>{/if}
	</div>
</form>

<!-- Items -->
<ul class="mt-6 divide-y rounded border">
	{#each data.items as item (item.id)}
		<li class="p-3">
			<div class="flex items-start justify-between gap-3">
				<div>
					{#if item.url}
						<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external, user-provided product URL -->
						<a href={item.url} class="font-medium hover:underline" rel="noreferrer" target="_blank"
							>{item.name}</a
						>
					{:else}
						<span class="font-medium">{item.name}</span>
					{/if}
					<div class="mt-0.5 text-xs text-gray-500">
						Wants {item.quantity_wanted ?? 1} · {availabilityLabel[
							item.availability ?? 'available'
						]}
						{#if (item.reserved_quantity ?? 0) > 0}
							· {item.reserved_quantity} reserved
						{/if}
					</div>
				</div>
				<form method="post" action="?/delete">
					<input type="hidden" name="item_id" value={item.id} />
					<button class="text-sm text-red-600 hover:underline">Delete</button>
				</form>
			</div>

			<!-- Inline edit -->
			<form method="post" action="?/edit" class="mt-2 flex items-center gap-2">
				<input type="hidden" name="item_id" value={item.id} />
				<input class="flex-1 rounded border p-1 text-sm" name="name" value={item.name} />
				<input
					class="w-16 rounded border p-1 text-sm"
					name="quantity_wanted"
					type="number"
					min="1"
					value={item.quantity_wanted ?? 1}
				/>
				<button class="rounded border px-2 py-1 text-sm hover:bg-gray-50">Save</button>
			</form>
		</li>
	{:else}
		<li class="p-3 text-gray-500">No items yet — add one above.</li>
	{/each}
</ul>
