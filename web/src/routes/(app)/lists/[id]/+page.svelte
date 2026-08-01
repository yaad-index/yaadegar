<script lang="ts">
	import { superForm } from 'sveltekit-superforms';
	import { enhance as formEnhance } from '$app/forms';
	import { resolve } from '$app/paths';
	import { replaceState } from '$app/navigation';
	import { page } from '$app/state';
	import Tabs from '$lib/components/Tabs.svelte';
	import type { PageData, ActionData } from './$types';

	let { data, form: actionForm }: { data: PageData; form: ActionData } = $props();
	// Import result surfaced from the ?/import action (aliased so it doesn't collide
	// with the superForm `form` store below).
	const imported = $derived(
		actionForm && 'imported' in actionForm ? actionForm.imported : undefined
	);
	const importError = $derived(
		actionForm && 'importError' in actionForm ? actionForm.importError : undefined
	);
	const importRowErrors = $derived<{ row: number; message: string }[]>(
		actionForm && 'importRowErrors' in actionForm
			? ((actionForm.importRowErrors as { row: number; message: string }[]) ?? [])
			: []
	);
	// superForm captures the initial form once and owns its reactivity thereafter.
	// svelte-ignore state_referenced_locally
	const { form, errors, message, enhance, submitting } = superForm(data.addForm);

	// Which tab is active. Local reactive state, bound into <Tabs>, so a click
	// switches the panel immediately — the visible panel keys off this state and does
	// NOT depend on the URL re-deriving (the earlier version relied on a shallow-routing
	// URL update to re-fire a derived value, which it does not, so clicks were inert).
	// Initialized from the URL (?tab=settings) so a reload or a shared link lands on
	// the same tab; default is the List (items) view (#128).
	let activeTab = $state(page.url.searchParams.get('tab') === 'settings' ? 'settings' : 'list');
	// Mirror the active tab into the URL for deep-link / reload persistence, without a
	// navigation (so in-progress form state survives a tab switch). <Tabs> owns the
	// immediate visual switch; this only keeps the URL in step.
	function mirrorTabToUrl(tab: string) {
		const url = new URL(page.url);
		if (tab === 'settings') url.searchParams.set('tab', 'settings');
		else url.searchParams.delete('tab');
		// resolve() can't see through the runtime query, and this never leaves the route.
		// eslint-disable-next-line svelte/no-navigation-without-resolve
		replaceState(url, {});
	}
	// Each tab carries a real href so a no-JS click is a normal navigation that still
	// lands on the right tab (the panel is chosen from the URL on load).
	function tabHref(tab: 'list' | 'settings') {
		const base = resolve('/(app)/lists/[id]', { id: data.list.id ?? '' });
		return tab === 'settings' ? `${base}?tab=settings` : base;
	}
	const listTabs = $derived([
		{ id: 'list', label: 'List', href: tabHref('list') },
		{ id: 'settings', label: 'Settings', href: tabHref('settings') }
	]);

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

<!-- Tabs: List (items — the primary view) and Settings (list-level config +
     import/export). The visible panel keys off `activeTab` (bound into <Tabs>), so a
     click switches immediately; the choice is mirrored to ?tab=settings for a
     deep-linkable / reload-stable URL (#128). -->
<Tabs tabs={listTabs} bind:active={activeTab} onselect={mirrorTabToUrl} label="List sections" />

{#if activeTab === 'settings'}
	<!-- List settings: reserver tier (#126) + co-buy default (#100) + thank-you note
	     default (#22). allow_cobuy and thank_you are overridable per item; the
	     reserver tier is a list-level setting only. -->
	<section class="mt-3 rounded border p-3">
		<p class="text-sm font-medium">List settings</p>
		<form method="post" action="?/settings" use:formEnhance class="mt-2 space-y-3">
			<div class="flex items-center gap-2">
				<label class="text-sm text-gray-700" for="list-allow-cobuy">Group-buying default</label>
				<select id="list-allow-cobuy" name="allow_cobuy" class="rounded border p-1.5 text-sm">
					<option value="true" selected={data.list.allow_cobuy !== false}>Allowed</option>
					<option value="false" selected={data.list.allow_cobuy === false}>Not allowed</option>
				</select>
			</div>
			<div class="flex items-center gap-2">
				<label class="text-sm text-gray-700" for="list-reserver-tier">Who can reserve</label>
				<select id="list-reserver-tier" name="reserver_tier" class="rounded border p-1.5 text-sm">
					<!-- Empty value = inherit the instance default (null override, three-state). -->
					<option value="" selected={data.list.reserver_tier == null}>Inherit default</option>
					<option value="full_guest" selected={data.list.reserver_tier === 'full_guest'}
						>Anyone</option
					>
					<option value="email_confirmed" selected={data.list.reserver_tier === 'email_confirmed'}
						>Require email confirmation</option
					>
				</select>
			</div>
			<div>
				<label class="block text-sm text-gray-700" for="list-thank-you"
					>Thank-you note (default)</label
				>
				<textarea
					id="list-thank-you"
					name="thank_you_template"
					rows="2"
					class="mt-1 w-full rounded border p-2 text-sm"
					placeholder="Emailed to a giver after they reserve. Use {'{item}'} for the item name. Leave blank for no note."
					>{data.list.thank_you_template ?? ''}</textarea
				>
				<p class="mt-0.5 text-xs text-gray-500">
					One-way: the giver never sees your list, and you never learn who reserved.
				</p>
			</div>
			<button class="rounded bg-black px-3 py-1.5 text-sm text-white">Save settings</button>
		</form>
	</section>

	<!-- Import / export: back up or move the item catalog (#26). It never includes
	     who reserved — items only. -->
	<section class="mt-3 rounded border p-3">
		<p class="text-sm font-medium">Import / export</p>
		<p class="mt-0.5 text-xs text-gray-600">
			Back up your items or move them elsewhere. Never includes who reserved.
		</p>

		<form
			method="post"
			action="?/import"
			enctype="multipart/form-data"
			use:formEnhance
			class="mt-2 flex flex-wrap items-center gap-2"
		>
			<input
				type="file"
				name="file"
				accept=".json,.csv,application/json,text/csv"
				class="text-sm"
			/>
			<button class="rounded border px-3 py-1.5 text-sm hover:bg-gray-50">Import</button>
		</form>
		{#if imported !== undefined}
			<p class="mt-1 text-xs text-green-700" role="status">Imported {imported} item(s).</p>
		{/if}
		{#if importError}
			<p class="mt-1 text-xs text-red-600" role="alert">{importError}</p>
			{#if importRowErrors.length > 0}
				<ul class="mt-1 list-inside list-disc text-xs text-red-600">
					{#each importRowErrors as e (e.row)}
						<li>Row {e.row}: {e.message}</li>
					{/each}
				</ul>
			{/if}
		{/if}

		<div class="mt-2 flex gap-2">
			<!-- eslint-disable svelte/no-navigation-without-resolve -- resolved path + a
			     ?format query the rule can't see through; both are internal download routes. -->
			<a
				class="rounded border px-3 py-1.5 text-sm hover:bg-gray-50"
				href={`${resolve('/(app)/lists/[id]/export', { id: data.list.id ?? '' })}?format=json`}
				download>Export JSON</a
			>
			<a
				class="rounded border px-3 py-1.5 text-sm hover:bg-gray-50"
				href={`${resolve('/(app)/lists/[id]/export', { id: data.list.id ?? '' })}?format=csv`}
				download>Export CSV</a
			>
			<!-- eslint-enable svelte/no-navigation-without-resolve -->
		</div>
	</section>
{:else}
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
			<button
				type="button"
				class="rounded bg-black px-3 py-2 text-sm text-white"
				onclick={copyShare}
			>
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
		<!-- Paste a product link and fetch its details, or fill the fields manually. -->
		<div class="flex gap-2">
			<input
				class="flex-1 rounded border p-2"
				name="url"
				placeholder="Paste a product link (optional)"
				bind:value={$form.url}
			/>
			<button
				type="submit"
				formaction="?/preview"
				class="rounded border px-3 py-2 text-sm hover:bg-gray-50 disabled:opacity-50"
				disabled={$submitting}
			>
				Fetch details
			</button>
		</div>
		<!-- Scraped image + price ride along into the create; the owner reviews first. -->
		<input type="hidden" name="image_url" bind:value={$form.image_url} />
		<input type="hidden" name="price_minor" bind:value={$form.price_minor} />
		<input type="hidden" name="price_currency" bind:value={$form.price_currency} />
		{#if $form.image_url || $form.price_minor}
			<div class="flex items-center gap-2 text-sm text-gray-600">
				{#if $form.image_url}
					<img src={$form.image_url} alt="" class="h-10 w-10 rounded border object-cover" />
				{/if}
				{#if $form.price_minor && $form.price_currency}
					<span>{($form.price_minor / 100).toFixed(2)} {$form.price_currency}</span>
				{/if}
			</div>
		{/if}
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
			{#if $message}<span class="text-xs text-gray-600">{$message}</span>{/if}
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
						<label class="block text-sm text-gray-600">
							Group-buying
							<!-- Per-item co-buy override (#100). "Use list default" keeps the item
							     inheriting; switching to Allowed/Not allowed sets an explicit override. -->
							<select name="allow_cobuy" class="mt-1 w-full rounded border p-2 text-sm">
								<option value="" selected={item.allow_cobuy == null}>Use list default</option>
								<option value="true" selected={item.allow_cobuy === true}>Allowed</option>
								<option value="false" selected={item.allow_cobuy === false}>Not allowed</option>
							</select>
						</label>
						<div class="text-sm text-gray-600">
							<!-- Per-item thank-you override (#22). Checked = inherit the list default
							     (sends null); unchecked = use the textarea (blank = no note for this item). -->
							<label class="flex items-center gap-2">
								<input
									type="checkbox"
									name="thank_you_inherit"
									checked={item.thank_you_template == null}
								/>
								Use list default thank-you note
							</label>
							<textarea
								name="thank_you_template"
								rows="2"
								class="mt-1 w-full rounded border p-2 text-sm"
								placeholder="Override the thank-you note for this item ({'{item}'} = item name; blank = no note)"
								>{item.thank_you_template ?? ''}</textarea
							>
						</div>
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
{/if}
