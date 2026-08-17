<script lang="ts">
	import { superForm } from 'sveltekit-superforms';
	import { enhance as formEnhance } from '$app/forms';
	import { resolve } from '$app/paths';
	import { replaceState } from '$app/navigation';
	import { page } from '$app/state';
	import Tabs from '$lib/components/Tabs.svelte';
	import Button from '$lib/components/Button.svelte';
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
	// resetForm:false is load-bearing: the ?/preview action returns the scraped draft
	// (name/url/image/price) as a success result, and the library default (resetForm:
	// true) would wipe $form back to empty on every success — discarding the prefill
	// and the pasted URL. With it off, a preview keeps the fetched values; a real add
	// clears the form because the ?/add success returns a fresh empty form (server).
	// The add form's price is entered in major units and drives the hidden price_minor
	// the ?/add action reads (see the 100-subunit note on the server). null = no price.
	let priceAmount = $state<number | null>(null);
	// The selected reserver tier, bound so the registered-tier warning reacts live to
	// the choice (ADR-0012 Decision 5). Initialized from the list's current override.
	// svelte-ignore state_referenced_locally
	let reserverTier = $state(data.list.reserver_tier ?? '');
	// svelte-ignore state_referenced_locally
	const { form, errors, message, enhance, submitting } = superForm(data.addForm, {
		resetForm: false,
		// After any successful update, reflect the (possibly scraped or cleared) price
		// into the amount input: a ?/preview brings a scraped price; a successful ?/add
		// returns a fresh empty form.
		onUpdated({ form }) {
			priceAmount = form.data.price_minor != null ? form.data.price_minor / 100 : null;
		}
	});

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
	// Status-chip tints. The design specifies two states — Available (green) and
	// Reserved (amber) — so those follow the export; amber is not in the token set
	// yet (flagged). co_buying/purchased are not in the design; they get restrained
	// stand-ins (gold accent / neutral) pending the designer.
	const availabilityChip: Record<string, string> = {
		available: 'bg-green-tint text-green',
		reserved: 'bg-amber-50 text-amber-700',
		co_buying: 'bg-gold-tint text-gold',
		purchased: 'bg-surface-alt text-ink-muted'
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

<a href={resolve('/')} class="font-ui text-ui text-ink-muted transition-colors hover:text-ink"
	>← Your lists</a
>
<h1 class="mt-1 font-display text-panel text-ink-heading">{data.list.title}</h1>
{#if data.descriptionHtml}
	<!-- data.descriptionHtml is sanitized server-side (renderNote: marked → sanitize-html
	     tight allowlist); {@html} only ever touches this pre-sanitized field (#143, ADR-0006). -->
	<!-- eslint-disable svelte/no-at-html-tags -->
	<div class="prose prose-sm mt-2 max-w-none text-ink">{@html data.descriptionHtml}</div>
	<!-- eslint-enable svelte/no-at-html-tags -->
{/if}

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
			<div>
				<label class="block text-sm text-gray-700" for="list-description">Description</label>
				<textarea
					id="list-description"
					name="description"
					rows="3"
					maxlength="2000"
					class="mt-1 w-full rounded border p-2 text-sm"
					placeholder="Shown to givers at the top of your list. Light markdown (bold, italic, links, lists). Leave blank for none."
					>{data.list.description ?? ''}</textarea
				>
				<p class="mt-0.5 text-xs text-gray-500">
					Supports light markdown; links open in a new tab. Max 2000 characters.
				</p>
			</div>
			<div class="flex items-center gap-2">
				<label class="text-sm text-gray-700" for="list-allow-cobuy">Group-buying default</label>
				<select id="list-allow-cobuy" name="allow_cobuy" class="rounded border p-1.5 text-sm">
					<option value="true" selected={data.list.allow_cobuy !== false}>Allowed</option>
					<option value="false" selected={data.list.allow_cobuy === false}>Not allowed</option>
				</select>
			</div>
			<div>
				<div class="flex items-center gap-2">
					<label class="text-sm text-gray-700" for="list-reserver-tier">Who can reserve</label>
					<!-- bind:value drives the live warning below; the value still posts as
					     reserver_tier. Empty = inherit the instance default (null, three-state). -->
					<select
						id="list-reserver-tier"
						name="reserver_tier"
						bind:value={reserverTier}
						class="rounded border p-1.5 text-sm"
					>
						<option value="">Inherit default</option>
						<option value="full_guest">Anyone</option>
						<option value="email_confirmed">Require email confirmation</option>
						<option value="registered">Require an account</option>
					</select>
				</div>
				{#if reserverTier === 'registered' && !data.registrationEnabled}
					<p class="mt-1 rounded bg-amber-50 p-2 text-xs text-amber-800" role="status">
						Self-registration is disabled on this instance, so only operator-created accounts can
						reserve on this list.
					</p>
				{/if}
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
	<!-- Share panel on a rose tint. The selectable input is always the fallback (works with
	     no JS and in non-secure contexts); the Copy button is a progressive enhancement.
	     shareUrl derives from the request origin + the /l/<slug> route — NOT a hardcoded
	     domain (the export's yaadegar.app/list/… is a mock; #209 gotcha 2). -->
	<section class="mt-4 rounded-card bg-primary-tint p-4">
		<p class="font-ui text-ui font-medium text-ink">Share this list</p>
		<p class="mt-1 font-ui text-ui text-ink-muted">
			Send this link to givers — they can reserve without an account, and you never see who reserved
			what.
		</p>
		<div class="mt-3 flex gap-2">
			<input
				class="h-12 flex-1 rounded-card border border-line bg-surface px-3 font-ui text-ui text-ink focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
				readonly
				value={shareUrl}
				aria-label="Public share link"
				onclick={(e) => e.currentTarget.select()}
			/>
			<Button type="button" onclick={copyShare}>Copy</Button>
		</div>
		{#if copied === 'ok'}
			<p class="mt-1 font-ui text-ui text-green" role="status">Link copied.</p>
		{:else if copied === 'fail'}
			<p class="mt-1 font-ui text-ui text-ink-muted" role="status">
				Couldn't copy automatically — select the link above and copy it.
			</p>
		{/if}
	</section>

	<!-- Add an item. Currency stays a free-text 3-letter field: the design shows a select,
	     but that would narrow the accepted values (a behaviour change, out of scope for a
	     visual PR) — flagged for the designer. All field names/actions/bindings unchanged. -->
	<form
		method="post"
		action="?/add"
		use:enhance
		class="mt-6 space-y-3 rounded-card border border-line bg-surface p-4"
	>
		<p class="font-display text-title text-ink-heading">Add an item</p>
		<div class="flex gap-2">
			<input
				class="h-12 flex-1 rounded-card border border-line bg-surface px-3 font-ui text-body text-ink placeholder:text-ink-muted focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
				name="name"
				placeholder="Item name"
				bind:value={$form.name}
			/>
			<input
				class="h-12 w-20 rounded-card border border-line bg-surface px-3 font-ui text-body text-ink focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
				name="quantity_wanted"
				type="number"
				min="1"
				aria-label="Quantity"
				bind:value={$form.quantity_wanted}
			/>
		</div>
		<!-- Paste a product link and fetch its details, or fill the fields manually. -->
		<div class="flex gap-2">
			<input
				class="h-12 flex-1 rounded-card border border-line bg-surface px-3 font-ui text-body text-ink placeholder:text-ink-muted focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
				name="url"
				placeholder="Paste a product link (optional)"
				bind:value={$form.url}
			/>
			<Button type="submit" formaction="?/preview" variant="secondary" disabled={$submitting}>
				Fetch
			</Button>
		</div>
		<!-- Editable price (major units) + currency: prefilled from a scrape but editable or
		     clearable. The amount drives the hidden price_minor the ?/add action reads. The
		     scraped image rides along read-only as a thumbnail. -->
		<input type="hidden" name="image_url" bind:value={$form.image_url} />
		<input
			type="hidden"
			name="price_minor"
			value={priceAmount == null ? '' : Math.round(priceAmount * 100)}
		/>
		<div class="flex flex-wrap items-center gap-2">
			<input
				class="h-12 w-28 rounded-card border border-line bg-surface px-3 font-ui text-body text-ink focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
				type="number"
				step="0.01"
				min="0"
				inputmode="decimal"
				placeholder="Price"
				aria-label="Price"
				bind:value={priceAmount}
			/>
			<input
				class="h-12 w-20 rounded-card border border-line bg-surface px-3 font-ui text-body uppercase text-ink focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
				name="price_currency"
				maxlength="3"
				placeholder="USD"
				aria-label="Currency"
				bind:value={$form.price_currency}
			/>
			{#if priceAmount != null && !$form.price_currency}
				<span class="font-ui text-ui text-red-600">Add a 3-letter currency for the price.</span>
			{/if}
			{#if $form.image_url}
				<img
					src={$form.image_url}
					alt=""
					class="h-10 w-10 rounded-card border border-line object-cover"
				/>
			{/if}
		</div>
		<textarea
			class="w-full rounded-card border border-line bg-surface p-3 font-ui text-body text-ink placeholder:text-ink-muted focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
			name="note"
			rows="2"
			placeholder="Note (optional — supports light markdown)"
			bind:value={$form.note}></textarea>
		<div class="flex items-center gap-3">
			<Button type="submit" disabled={$submitting}>Add item</Button>
			{#if $errors.name}<span class="font-ui text-ui text-red-600">{$errors.name}</span>{/if}
			{#if $errors.url}<span class="font-ui text-ui text-red-600">{$errors.url}</span>{/if}
			{#if $message}<span class="font-ui text-ui text-ink-muted">{$message}</span>{/if}
		</div>
	</form>

	<!-- Your items -->
	<h2 class="mt-8 font-display text-title text-ink-heading">Your items</h2>
	<ul class="mt-3 space-y-3">
		{#each data.items as item (item.id)}
			{@const id = item.id ?? ''}
			{@const availability = item.availability ?? 'available'}
			<li class="rounded-card border border-line bg-surface p-4">
				<div class="flex items-start justify-between gap-3">
					<div class="flex min-w-0 gap-3">
						{#if item.image_url}
							<img
								src={item.image_url}
								alt={item.name}
								class="h-14 w-14 shrink-0 rounded-card border border-line object-cover"
							/>
						{/if}
						<div class="min-w-0">
							<div class="flex flex-wrap items-center gap-2">
								{#if item.url}
									<!-- eslint-disable svelte/no-navigation-without-resolve -- external, user-provided product URL -->
									<a
										href={item.url}
										class="font-ui text-body font-medium text-ink-heading hover:underline"
										rel="noreferrer"
										target="_blank">{item.name}</a
									>
									<!-- eslint-enable svelte/no-navigation-without-resolve -->
								{:else}
									<span class="font-ui text-body font-medium text-ink-heading">{item.name}</span>
								{/if}
								<!-- The owner sees THAT an item is reserved, never who (ADR-0002 §5) —
								     reserved-state, not reserver-identity (#209 gotcha 3). -->
								<span
									class={`rounded-card px-2 py-0.5 font-ui text-chip ${availabilityChip[availability]}`}
								>
									{availabilityLabel[availability]}
								</span>
							</div>
							<div class="mt-1 font-ui text-ui text-ink-muted">
								Wants {item.quantity_wanted ?? 1}{#if (item.reserved_quantity ?? 0) > 0}
									· {item.reserved_quantity}
									reserved{/if}
							</div>
							{#if data.noteHtml[id]}
								<!-- data.noteHtml is sanitized server-side (marked → sanitize-html tight
								     allowlist); {@html} only ever touches this pre-sanitized field. -->
								<!-- eslint-disable svelte/no-at-html-tags -->
								<div class="prose prose-sm mt-1 max-w-none text-ink">
									{@html data.noteHtml[id]}
								</div>
								<!-- eslint-enable svelte/no-at-html-tags -->
							{/if}
						</div>
					</div>
					<div class="flex shrink-0 gap-3 font-ui text-ui">
						<button
							type="button"
							class="text-ink-muted transition-colors hover:text-ink"
							onclick={() => (editingId = editingId === id ? null : id)}
						>
							{editingId === id ? 'Close' : 'Edit'}
						</button>
						<form method="post" action="?/delete" use:formEnhance>
							<input type="hidden" name="item_id" value={id} />
							<button class="text-red-600 transition-colors hover:text-red-700">Delete</button>
						</form>
					</div>
				</div>

				{#if editingId === id}
					<form
						method="post"
						action="?/edit"
						use:formEnhance={onEditSubmit}
						class="mt-4 space-y-3 rounded-card border border-line bg-surface-alt p-4"
					>
						<p class="font-ui text-chip font-medium uppercase tracking-wide text-ink-muted">
							Editing item
						</p>
						<input type="hidden" name="item_id" value={id} />
						<div class="flex gap-2">
							<input
								class="h-12 flex-1 rounded-card border border-line bg-surface px-3 font-ui text-body text-ink focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
								name="name"
								aria-label="Item name"
								value={item.name}
							/>
							<input
								class="h-12 w-20 rounded-card border border-line bg-surface px-3 font-ui text-body text-ink focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
								name="quantity_wanted"
								type="number"
								min="1"
								aria-label="Quantity"
								value={item.quantity_wanted ?? 1}
							/>
						</div>
						<input
							class="h-12 w-full rounded-card border border-line bg-surface px-3 font-ui text-body text-ink placeholder:text-ink-muted focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
							name="url"
							placeholder="Link (optional)"
							value={item.url ?? ''}
						/>
						<!-- Editable price (major units) + currency, prefilled from the item.
						     Set-if-present: ?/edit includes price only when an amount is entered;
						     clearing it back to none isn't supported by the current backend PATCH,
						     the same limitation as the other edit fields. -->
						<div class="flex gap-2">
							<input
								class="h-12 w-28 rounded-card border border-line bg-surface px-3 font-ui text-body text-ink focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
								name="price_amount"
								type="number"
								step="0.01"
								min="0"
								inputmode="decimal"
								placeholder="Price"
								aria-label="Price"
								value={item.price?.amount_minor != null ? item.price.amount_minor / 100 : ''}
							/>
							<input
								class="h-12 w-20 rounded-card border border-line bg-surface px-3 font-ui text-body uppercase text-ink focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
								name="price_currency"
								maxlength="3"
								placeholder="USD"
								aria-label="Currency"
								value={item.price?.currency ?? ''}
							/>
						</div>
						<textarea
							class="w-full rounded-card border border-line bg-surface p-3 font-ui text-body text-ink placeholder:text-ink-muted focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
							name="note"
							rows="3"
							placeholder="Note (optional — supports light markdown)">{item.note ?? ''}</textarea
						>
						<label class="block font-ui text-ui text-ink-muted">
							Group-buying
							<!-- Per-item co-buy override (#100). "Use list default" keeps the item
							     inheriting; switching to Allowed/Not allowed sets an explicit override. -->
							<select
								name="allow_cobuy"
								class="mt-1 h-12 w-full rounded-card border border-line bg-surface px-3 font-ui text-body text-ink"
							>
								<option value="" selected={item.allow_cobuy == null}>Use list default</option>
								<option value="true" selected={item.allow_cobuy === true}>Allowed</option>
								<option value="false" selected={item.allow_cobuy === false}>Not allowed</option>
							</select>
						</label>
						<div class="font-ui text-ui text-ink-muted">
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
								class="mt-1 w-full rounded-card border border-line bg-surface p-3 font-ui text-body text-ink placeholder:text-ink-muted focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
								placeholder="Override the thank-you note for this item ({'{item}'} = item name; blank = no note)"
								>{item.thank_you_template ?? ''}</textarea
							>
						</div>
						<div class="flex gap-2">
							<Button type="submit">Save changes</Button>
							<Button type="button" variant="secondary" onclick={() => (editingId = null)}>
								Cancel
							</Button>
						</div>
					</form>
				{/if}
			</li>
		{:else}
			<li
				class="rounded-card border border-line bg-surface p-6 text-center font-ui text-body text-ink-muted"
			>
				No items yet — add one above.
			</li>
		{/each}
	</ul>
{/if}
