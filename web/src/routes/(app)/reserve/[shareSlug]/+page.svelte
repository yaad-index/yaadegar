<script lang="ts">
	import { enhance } from '$app/forms';
	import { resolve } from '$app/paths';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();

	// Availability is the only reservation signal about OTHER givers (ADR-0002
	// anonymity): "reserved by you" comes from the account's own dashboard, never this.
	const availabilityLabel: Record<string, string> = {
		available: 'Available',
		reserved: 'Reserved',
		co_buying: 'Being co-bought',
		purchased: 'Purchased'
	};

	function fmt(m: { amount_minor?: number; currency?: string } | null | undefined): string {
		if (!m || typeof m.amount_minor !== 'number' || !m.currency) return '';
		return `${(m.amount_minor / 100).toFixed(2)} ${m.currency}`;
	}

	const reserveError = $derived(form && 'reserveError' in form ? form.reserveError : undefined);
	const releaseError = $derived(form && 'releaseError' in form ? form.releaseError : undefined);
	const reserved = $derived(form && 'reserved' in form ? form.reserved : undefined);
</script>

<svelte:head>
	<title>{data.closed ? 'List unavailable' : (data.list?.title ?? 'Reserve')} · Yaadegar</title>
</svelte:head>

<main class="mx-auto max-w-2xl p-6">
	{#if data.closed}
		<h1 class="text-2xl font-bold">This list is no longer active</h1>
		<p class="mt-3 text-gray-600">
			The owner has closed it, or its event date has passed. There's nothing to reserve here right
			now.
		</p>
		<a
			class="mt-4 inline-block text-sm text-gray-600 underline"
			href={resolve('/(app)/reservations')}>Back to your reservations</a
		>
	{:else if data.list}
		<h1 class="text-2xl font-bold">{data.list.title}</h1>
		<p class="mt-1 text-sm text-gray-600">
			Reserving here is tied to your account, so it shows up in your reservations. The list owner
			never learns who reserved.
		</p>
		{#if data.descriptionHtml}
			<!-- data.descriptionHtml is sanitized server-side (renderNote); {@html} only ever
			     touches this pre-sanitized field (#143, ADR-0006). -->
			<!-- eslint-disable svelte/no-at-html-tags -->
			<div class="prose prose-sm mt-2 max-w-none text-gray-700">{@html data.descriptionHtml}</div>
			<!-- eslint-enable svelte/no-at-html-tags -->
		{/if}
		{#if data.list.event_date}
			<p class="mt-1 text-sm text-gray-600">For {data.list.event_date}</p>
		{/if}

		{#if reserved}
			<p class="mt-4 rounded bg-green-50 p-2 text-sm text-green-800" role="status">
				Reserved — it's in your reservations.
			</p>
		{/if}
		{#if reserveError}
			<p class="mt-4 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">{reserveError}</p>
		{/if}
		{#if releaseError}
			<p class="mt-4 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">{releaseError}</p>
		{/if}

		<form method="post" use:enhance>
			<ul class="mt-6 divide-y rounded border">
				{#each data.list.items ?? [] as item (item.id)}
					{@const mine = item.id ? data.reservedItems[item.id] : undefined}
					<li class="p-4">
						<div class="flex items-start justify-between gap-4">
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
										<!-- data.noteHtml is sanitized server-side; {@html} only touches it. -->
										<!-- eslint-disable svelte/no-at-html-tags -->
										<div class="prose prose-sm mt-0.5 max-w-none text-sm text-gray-600">
											{@html data.noteHtml[item.id]}
										</div>
										<!-- eslint-enable svelte/no-at-html-tags -->
									{/if}
									<p class="mt-1 flex flex-wrap gap-x-3 text-sm text-gray-500">
										{#if fmt(item.price)}<span>{fmt(item.price)}</span>{/if}
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
								{#if mine}
									<p class="text-sm font-medium text-green-700">✓ You reserved this</p>
									<button
										class="mt-1 text-sm text-gray-600 underline"
										formaction="?/release"
										name="reservation_id"
										value={mine.reservation_id}>Release</button
									>
								{:else if item.availability === 'available'}
									<button
										class="rounded bg-black px-3 py-2 text-sm text-white"
										formaction="?/reserve"
										name="item_id"
										value={item.id}>Reserve it</button
									>
								{:else}
									<span class="text-sm text-gray-400"
										>{availabilityLabel[item.availability ?? ''] ?? 'Taken'}</span
									>
								{/if}
							</div>
						</div>
					</li>
				{/each}
			</ul>
		</form>

		<p class="mt-6 text-xs text-gray-500">
			Reserving keeps the surprise: the owner never sees who reserved.
		</p>
	{/if}
</main>
