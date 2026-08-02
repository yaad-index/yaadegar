<script module lang="ts">
	// reserveNeedsEmail is the client-side guard for the reserve submit (#144): on an
	// email-confirm list the backend rejects a reservation with no giver email, so the
	// UI blocks that submit up front. Only the reserve action is gated — release,
	// withdraw, refresh, and pledge (which has its own email field) submit freely.
	// Extracted so the rule is unit-testable without driving `enhance` in the DOM.
	export function reserveNeedsEmail(
		actionSearch: string,
		emailRequired: boolean,
		email: string
	): boolean {
		return actionSearch === '?/reserve' && emailRequired && email.trim() === '';
	}
</script>

<script lang="ts">
	import { enhance } from '$app/forms';
	import type { SubmitFunction } from '@sveltejs/kit';
	import { resolve } from '$app/paths';
	import { chipInAllowed } from '$lib/cobuy';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();

	// Availability is the only reservation signal a giver sees — never who reserved
	// or how many people did (ADR-0002 anonymity). "reserved by you" / "chipping in"
	// are derived separately from this browser's own capability cookies, not the list.
	const availabilityLabel: Record<string, string> = {
		available: 'Available',
		reserved: 'Reserved',
		co_buying: 'Being co-bought',
		purchased: 'Purchased'
	};

	type Money = { amount_minor?: number; currency?: string } | null | undefined;

	function fmt(m: Money): string {
		if (!m || typeof m.amount_minor !== 'number' || !m.currency) return '';
		return `${(m.amount_minor / 100).toFixed(2)} ${m.currency}`;
	}
	const major = (m: Money): number =>
		m && typeof m.amount_minor === 'number' ? m.amount_minor / 100 : 0;

	// Which item's "Chip in" form is currently open (only one at a time), plus the
	// amount bound to that form so the share-preset buttons can fill it.
	let openPledge = $state<string | null>(null);
	let pledgeAmount = $state('');

	function openChipIn(itemId: string) {
		openPledge = itemId;
		pledgeAmount = '';
	}
	function setShare(price: number, fraction: number) {
		pledgeAmount = (price * fraction).toFixed(2);
	}

	// Feedback surfaced from each action; distinct keys keep reserve, pledge, and
	// withdraw messages from colliding on the shared `form` prop.
	const reserveMessage = $derived(form && 'form' in form ? form.form?.message : undefined);
	const emailError = $derived(
		form && 'form' in form ? form.form?.errors?.giver_email?.[0] : undefined
	);
	const releaseError = $derived(form && 'releaseError' in form ? form.releaseError : undefined);
	const pledgeMessage = $derived(form && 'pledgeMessage' in form ? form.pledgeMessage : undefined);
	const pledgeError = $derived(form && 'pledgeError' in form ? form.pledgeError : undefined);
	const amountError = $derived(
		form && 'pledgeForm' in form ? form.pledgeForm?.errors?.amount?.[0] : undefined
	);
	const contactError = $derived(
		form && 'pledgeForm' in form ? form.pledgeForm?.errors?.contact_email?.[0] : undefined
	);
	const withdrawError = $derived(form && 'withdrawError' in form ? form.withdrawError : undefined);

	// email_required (#144): an email-confirm list rejects a reservation with no giver
	// email server-side. Mirror that in the UI — mark the email field required and block
	// the reserve submit up front so the giver sees the requirement instead of a failed
	// round-trip. Name stays genuinely optional; release/withdraw/pledge are unaffected.
	const emailRequired = $derived(!data.closed && !!data.list?.email_required);
	let giverEmail = $state('');
	let clientEmailError = $state<string | null>(null);

	const guardReserve: SubmitFunction = ({ action, cancel }) => {
		if (reserveNeedsEmail(action.search, emailRequired, giverEmail)) {
			clientEmailError = 'Enter your email to reserve on this list.';
			cancel();
			return;
		}
		clientEmailError = null;
	};
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
		{#if pledgeMessage}
			<p class="mt-4 rounded bg-green-50 p-2 text-sm text-green-800" role="status">
				{pledgeMessage}
			</p>
		{/if}
		{#if emailError}
			<p class="mt-4 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">{emailError}</p>
		{/if}
		{#if clientEmailError}
			<p class="mt-4 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">
				{clientEmailError}
			</p>
		{/if}
		{#if releaseError}
			<p class="mt-4 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">{releaseError}</p>
		{/if}
		{#if withdrawError}
			<p class="mt-4 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">{withdrawError}</p>
		{/if}

		<!-- One form drives reserve/release/pledge/withdraw: each button carries its own
		     item_id and targets its action via formaction. The optional giver identity
		     for a full reserve is entered once and applies to whichever item is reserved. -->
		<form method="post" use:enhance={guardReserve} class="mt-6">
			<fieldset class="rounded border p-4">
				<legend class="px-1 text-sm font-medium text-gray-700">
					{emailRequired ? 'Your details' : 'Your details (optional)'}
				</legend>
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
						<span class="text-sm">{emailRequired ? 'Email (required)' : 'Email'}</span>
						<input
							class="mt-1 w-full rounded border p-2"
							name="giver_email"
							type="email"
							autocomplete="email"
							bind:value={giverEmail}
							aria-required={emailRequired}
							placeholder={emailRequired
								? 'Required to reserve on this list'
								: 'For reminders only'}
						/>
					</label>
				</div>
				<p class="mt-2 text-xs text-gray-500">
					{#if emailRequired}
						This list needs your email to confirm your reservation — never shown to the list owner
						or other givers. Your name is optional.
					{:else}
						Used only to remind you — never shown to the list owner or other givers.
					{/if}
				</p>
			</fieldset>

			<ul class="mt-6 divide-y rounded border">
				{#each data.list.items ?? [] as item (item.id)}
					{@const reservedByYou = !!item.id && data.reservedItemIds.includes(item.id)}
					{@const pledge = item.id ? data.pledged[item.id] : undefined}
					{@const priceMajor = major(item.price)}
					{@const fundedMajor = major(item.amount_funded)}
					{@const remaining = Math.max(priceMajor - fundedMajor, 0)}
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
										<!-- data.noteHtml is sanitized server-side (marked → sanitize-html tight
										     allowlist); {@html} only ever touches this pre-sanitized field. -->
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

									{#if item.availability === 'co_buying'}
										<!-- Co-buy progress: a state + a funded amount only, never who chipped in. -->
										<div class="mt-2 max-w-xs">
											<div class="h-2 overflow-hidden rounded bg-gray-200">
												<div
													class="h-full bg-green-500"
													style="width: {priceMajor > 0
														? Math.min(100, Math.round((fundedMajor / priceMajor) * 100))
														: 0}%"
												></div>
											</div>
											<p class="mt-1 text-xs text-gray-500">
												{fmt(item.amount_funded)} of {fmt(item.price)} chipped in
											</p>
										</div>
									{/if}
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
								{:else if pledge}
									<p class="text-sm font-medium text-green-700">✓ You're chipping in</p>
									{#if pledge.matched && pledge.match_id}
										<a
											class="mt-1 block text-sm text-blue-700 underline"
											href={resolve('/cobuy/[matchId]', { matchId: pledge.match_id })}
											>Confirm the group buy →</a
										>
									{:else}
										<button
											class="mt-1 block text-sm text-gray-600 underline"
											formaction="?/withdraw"
											name="item_id"
											value={item.id}>Withdraw pledge</button
										>
									{/if}
									<button class="mt-1 block text-xs text-gray-500 underline" formaction="?/refresh"
										>Check for updates</button
									>
								{:else if item.availability === 'available'}
									<button
										class="rounded bg-black px-3 py-2 text-sm text-white"
										formaction="?/reserve"
										name="item_id"
										value={item.id}>Reserve it</button
									>
									{#if chipInAllowed(item)}
										<button
											type="button"
											class="mt-1 block text-sm text-blue-700 underline"
											onclick={() => item.id && openChipIn(item.id)}>Chip in instead</button
										>
									{/if}
								{:else if item.availability === 'co_buying' && chipInAllowed(item)}
									<button
										type="button"
										class="rounded bg-black px-3 py-2 text-sm text-white"
										onclick={() => item.id && openChipIn(item.id)}>Chip in the rest</button
									>
								{:else}
									<span class="text-sm text-gray-400"
										>{availabilityLabel[item.availability ?? ''] ?? 'Taken'}</span
									>
								{/if}
							</div>
						</div>

						{#if openPledge === item.id && item.price && chipInAllowed(item)}
							<!-- Inline chip-in form for this item. Amount is in the item's currency
							     (hidden field); the backend rejects a currency mismatch. -->
							<div class="mt-3 rounded border bg-gray-50 p-3">
								<input type="hidden" name="item_id" value={item.id} />
								<input type="hidden" name="currency" value={item.price.currency} />
								<p class="text-sm font-medium">Chip in toward {item.name}</p>
								{#if remaining > 0}
									<p class="text-xs text-gray-500">
										{remaining.toFixed(2)}
										{item.price.currency} still needed
									</p>
								{/if}
								<div class="mt-2 flex flex-wrap items-center gap-2">
									<button
										type="button"
										class="rounded border px-2 py-1 text-xs"
										onclick={() => setShare(priceMajor, 1 / 2)}>½</button
									>
									<button
										type="button"
										class="rounded border px-2 py-1 text-xs"
										onclick={() => setShare(priceMajor, 1 / 3)}>⅓</button
									>
									<button
										type="button"
										class="rounded border px-2 py-1 text-xs"
										onclick={() => setShare(priceMajor, 1 / 4)}>¼</button
									>
									<label class="flex items-center gap-1 text-sm">
										<span>Amount</span>
										<input
											class="w-28 rounded border p-1"
											name="amount"
											type="number"
											min="0"
											step="0.01"
											bind:value={pledgeAmount}
										/>
										<span class="text-xs text-gray-500">{item.price.currency}</span>
									</label>
								</div>
								{#if amountError}<p class="mt-1 text-xs text-red-700" role="alert">
										{amountError}
									</p>{/if}
								<label class="mt-2 block">
									<span class="text-sm">Your email</span>
									<input
										class="mt-1 w-full rounded border p-2"
										name="contact_email"
										type="email"
										autocomplete="email"
										placeholder="Revealed only if the group buy is confirmed by everyone"
									/>
								</label>
								{#if contactError}<p class="mt-1 text-xs text-red-700" role="alert">
										{contactError}
									</p>{/if}
								{#if pledgeError}<p class="mt-1 text-xs text-red-700" role="alert">
										{pledgeError}
									</p>{/if}
								<p class="mt-1 text-xs text-gray-500">
									Your email is shown to your co-buyers only after everyone confirms — never to the
									owner.
								</p>
								<div class="mt-2 flex gap-2">
									<button
										class="rounded bg-black px-3 py-1.5 text-sm text-white"
										formaction="?/pledge">Pledge</button
									>
									<button
										type="button"
										class="text-sm text-gray-600 underline"
										onclick={() => (openPledge = null)}>Cancel</button
									>
								</div>
							</div>
						{/if}
					</li>
				{/each}
			</ul>
		</form>

		<p class="mt-6 text-xs text-gray-500">
			Reserving or chipping in keeps the surprise: the owner never sees who did either.
		</p>
	{/if}
</main>
