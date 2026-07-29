<script lang="ts">
	import { enhance } from '$app/forms';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();

	// Availability is the only reservation signal a giver sees — never who reserved
	// or how many people did (ADR-0002 anonymity). "reserved by you" is derived
	// separately from this browser's own capability cookie, not from the list.
	const availabilityLabel: Record<string, string> = {
		available: 'Available',
		reserved: 'Reserved',
		co_buying: 'Being co-bought',
		purchased: 'Purchased'
	};

	function price(m: { amount_minor?: number; currency?: string } | null | undefined): string {
		if (!m || typeof m.amount_minor !== 'number' || !m.currency) return '';
		return `${(m.amount_minor / 100).toFixed(2)} ${m.currency}`;
	}

	// Feedback surfaced from the reserve action (superforms message) / release action.
	const reserveMessage = $derived(form && 'form' in form ? form.form?.message : undefined);
	const emailError = $derived(
		form && 'form' in form ? form.form?.errors?.giver_email?.[0] : undefined
	);
	const releaseError = $derived(form && 'releaseError' in form ? form.releaseError : undefined);
</script>

<svelte:head>
	<title>{data.closed ? 'List unavailable' : (data.list?.title ?? 'Shared list')} · Yaadegar</title>
	{#if !data.closed && data.list}
		<meta property="og:title" content={data.list.title} />
		<meta property="og:description" content="A shared gift list on Yaadegar." />
	{/if}
</svelte:head>

<main class="mx-auto max-w-2xl p-6">
	{#if data.closed}
		<h1 class="text-2xl font-bold">This list is no longer active</h1>
		<p class="mt-3 text-gray-600">
			The owner has closed it, or its event date has passed. There's nothing to reserve here right
			now.
		</p>
	{:else if data.list}
		<h1 class="text-2xl font-bold">{data.list.title}</h1>
		{#if data.list.event_date}
			<p class="mt-1 text-sm text-gray-600">For {data.list.event_date}</p>
		{/if}

		{#if reserveMessage}
			<p class="mt-4 rounded bg-green-50 p-2 text-sm text-green-800" role="status">
				{reserveMessage}
			</p>
		{/if}
		{#if emailError}
			<p class="mt-4 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">{emailError}</p>
		{/if}
		{#if releaseError}
			<p class="mt-4 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">{releaseError}</p>
		{/if}

		<!-- One form drives both actions: each item's button carries its own item_id and
		     targets ?/reserve or ?/release via formaction. The optional giver identity is
		     entered once and applies to whichever item is reserved. -->
		<form method="post" use:enhance class="mt-6">
			<fieldset class="rounded border p-4">
				<legend class="px-1 text-sm font-medium text-gray-700">Your details (optional)</legend>
				<div class="grid gap-3 sm:grid-cols-2">
					<label class="block">
						<span class="text-sm">Name</span>
						<input
							class="mt-1 w-full rounded border p-2"
							name="giver_name"
							autocomplete="name"
							placeholder="Shown to no one"
						/>
					</label>
					<label class="block">
						<span class="text-sm">Email</span>
						<input
							class="mt-1 w-full rounded border p-2"
							name="giver_email"
							type="email"
							autocomplete="email"
							placeholder="For reminders only"
						/>
					</label>
				</div>
				<p class="mt-2 text-xs text-gray-500">
					Used only to remind you — never shown to the list owner or other givers.
				</p>
			</fieldset>

			<ul class="mt-6 divide-y rounded border">
				{#each data.list.items ?? [] as item (item.id)}
					{@const reservedByYou = !!item.id && data.reservedItemIds.includes(item.id)}
					<li class="flex items-start justify-between gap-4 p-4">
						<div class="flex min-w-0 gap-3">
							{#if item.image_url}
								<img
									src={item.image_url}
									alt={item.name}
									class="h-14 w-14 shrink-0 rounded border object-cover"
								/>
							{/if}
							<div class="min-w-0">
								<p class="font-medium">{item.name}</p>
								{#if item.id && data.noteHtml[item.id]}
									<!-- data.noteHtml is sanitized server-side (marked → sanitize-html tight
									     allowlist); {@html} only ever touches this pre-sanitized field. -->
									<!-- eslint-disable svelte/no-at-html-tags -->
									<div class="prose prose-sm mt-0.5 max-w-none text-sm text-gray-600">
										{@html data.noteHtml[item.id]}
									</div>
									<!-- eslint-enable svelte/no-at-html-tags -->
								{/if}
								<p class="mt-1 flex flex-wrap gap-x-3 text-sm text-gray-500">
									{#if price(item.price)}<span>{price(item.price)}</span>{/if}
									<span>{availabilityLabel[item.availability ?? ''] ?? 'Available'}</span>
									{#if item.url}
										<!-- eslint-disable svelte/no-navigation-without-resolve -- external, user-provided product URL -->
										<a
											href={item.url}
											class="text-blue-700 underline"
											rel="noreferrer"
											target="_blank">View item</a
										>
										<!-- eslint-enable svelte/no-navigation-without-resolve -->
									{/if}
								</p>
							</div>
						</div>
						<div class="shrink-0 text-right">
							{#if reservedByYou}
								<p class="text-sm font-medium text-green-700">✓ You reserved this</p>
								<button
									class="mt-1 text-sm text-gray-600 underline"
									formaction="?/release"
									name="item_id"
									value={item.id}>Release</button
								>
							{:else if item.availability === 'available'}
								<button
									class="rounded bg-black px-3 py-2 text-sm text-white"
									formaction="?/reserve"
									name="item_id"
									value={item.id}>Reserve</button
								>
							{:else}
								<span class="text-sm text-gray-400">Taken</span>
							{/if}
						</div>
					</li>
				{/each}
			</ul>
		</form>

		<p class="mt-6 text-xs text-gray-500">
			Reserving keeps the surprise: the owner never sees who reserved what.
		</p>
	{/if}
</main>
