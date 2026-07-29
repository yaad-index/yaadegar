<script lang="ts">
	import { superForm } from 'sveltekit-superforms';
	import { enhance as formEnhance } from '$app/forms';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	// superForm captures the initial form once and owns its reactivity thereafter.
	// svelte-ignore state_referenced_locally
	const { form, errors, message, enhance, submitting } = superForm(data.addForm);

	// The public giver link is this list's share slug on the current (tenant) origin.
	const shareUrl = $derived(`${page.url.origin}/l/${data.list.share_slug ?? ''}`);
	let copied = $state<'idle' | 'ok' | 'fail'>('idle');
	let resetTimer: ReturnType<typeof setTimeout> | undefined;
	async function copyShare() {
		clearTimeout(resetTimer);
		try {
			await navigator.clipboard.writeText(shareUrl);
			copied = 'ok';
		} catch {
			copied = 'fail';
		}
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

	// Which item's editor is open (only one at a time).
	let editingId = $state<string | null>(null);

	// Close the editor once its save succeeds; otherwise keep it open with the error.
	const onEditSubmit = () => {
		return async ({
			result,
			update
		}: {
			result: { type: string };
			update: () => Promise<void>;
		}) => {
			await update();
			if (result.type === 'success') editingId = null;
		};
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
	<textarea
		class="w-full rounded border p-2"
		name="note"
		rows="2"
		placeholder="Note (optional — supports light markdown)"
		bind:value={$form.note}></textarea>
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
		{@const id = item.id ?? ''}
		<li class="p-3">
			<div class="flex items-start justify-between gap-3">
				<div class="flex min-w-0 gap-3">
					{#if item.image_url}
						<img
							src={item.image_url}
							alt={item.name}
							class="h-14 w-14 shrink-0 rounded border object-cover"
						/>
					{/if}
					<div class="min-w-0">
						{#if item.url}
							<!-- eslint-disable svelte/no-navigation-without-resolve -- external, user-provided product URL -->
							<a
								href={item.url}
								class="font-medium hover:underline"
								rel="noreferrer"
								target="_blank">{item.name}</a
							>
							<!-- eslint-enable svelte/no-navigation-without-resolve -->
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
						{#if data.noteHtml[id]}
							<!-- data.noteHtml is sanitized server-side (marked → sanitize-html tight
							     allowlist); {@html} only ever touches this pre-sanitized field. -->
							<!-- eslint-disable svelte/no-at-html-tags -->
							<div class="prose prose-sm mt-1 max-w-none text-sm text-gray-700">
								{@html data.noteHtml[id]}
							</div>
							<!-- eslint-enable svelte/no-at-html-tags -->
						{/if}
					</div>
				</div>
				<div class="flex shrink-0 gap-3 text-sm">
					<button
						type="button"
						class="text-gray-600 hover:underline"
						onclick={() => (editingId = editingId === id ? null : id)}
					>
						{editingId === id ? 'Close' : 'Edit'}
					</button>
					<form method="post" action="?/delete" use:formEnhance>
						<input type="hidden" name="item_id" value={id} />
						<button class="text-red-600 hover:underline">Delete</button>
					</form>
				</div>
			</div>

			{#if editingId === id}
				<form method="post" action="?/edit" use:formEnhance={onEditSubmit} class="mt-3 space-y-2">
					<input type="hidden" name="item_id" value={id} />
					<div class="flex gap-2">
						<input class="flex-1 rounded border p-2 text-sm" name="name" value={item.name} />
						<input
							class="w-20 rounded border p-2 text-sm"
							name="quantity_wanted"
							type="number"
							min="1"
							value={item.quantity_wanted ?? 1}
						/>
					</div>
					<input
						class="w-full rounded border p-2 text-sm"
						name="url"
						placeholder="Link (optional)"
						value={item.url ?? ''}
					/>
					<textarea
						class="w-full rounded border p-2 text-sm"
						name="note"
						rows="3"
						placeholder="Note (optional — supports light markdown)">{item.note ?? ''}</textarea
					>
					<div class="flex gap-2">
						<button class="rounded bg-black px-3 py-1.5 text-sm text-white">Save</button>
						<button
							type="button"
							class="rounded border px-3 py-1.5 text-sm hover:bg-gray-50"
							onclick={() => (editingId = null)}>Cancel</button
						>
					</div>
				</form>
			{/if}
		</li>
	{:else}
		<li class="p-3 text-gray-500">No items yet — add one above.</li>
	{/each}
</ul>
