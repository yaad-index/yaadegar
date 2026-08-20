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
	import CaptchaWidget from '$lib/components/CaptchaWidget.svelte';
	import Button from '$lib/components/Button.svelte';
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
	// Availability chip tints — Available (green) and Reserved (amber) follow the export
	// and match the owner list view; co_buying/purchased get restrained stand-ins pending
	// the designer (amber is not a brand token yet, flagged with the list view).
	const availabilityChip: Record<string, string> = {
		available: 'bg-green-tint text-green',
		reserved: 'bg-amber-50 text-amber-700',
		co_buying: 'bg-gold-tint text-gold',
		purchased: 'bg-surface-alt text-ink-muted'
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

	// A single generic item glyph. The export draws a per-item line glyph (coffee/book/
	// ribbon) in a tinted circle, but the payload carries no category to choose one and
	// #234 draws photographs for the same items — both logged as design questions
	// (design-open-questions.md), so this renders one honest generic mark rather than a
	// photo or an invented category. Amber-tinted when this browser reserved the item.
</script>

{#snippet giftGlyph()}
	<svg
		width="24"
		height="24"
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="2"
		stroke-linecap="round"
		stroke-linejoin="round"
		aria-hidden="true"
	>
		<rect x="3" y="8" width="18" height="4" rx="1" />
		<path d="M12 8v13M5 12v7a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-7" />
		<path d="M12 8S9.5 3.5 7.5 4.5 7 8 12 8Zm0 0s2.5-4.5 4.5-3.5S17 8 12 8Z" />
	</svg>
{/snippet}

<svelte:head>
	<title>{data.closed ? 'List unavailable' : (data.list?.title ?? 'Shared list')} · Yaadegar</title>
	{#if !data.closed && data.list}
		<meta property="og:title" content={data.list.title} />
		<meta property="og:description" content="A shared gift list on Yaadegar." />
	{/if}
</svelte:head>

<!-- Post-reserve / post-pledge success is a full-width banner ABOVE the header, not a
     box inside the page column (per the export). -->
{#if reserveMessage}
	<div
		class="w-full bg-green-50 py-3 text-center font-ui text-ui font-medium text-green-800"
		role="status"
	>
		✓ {reserveMessage}
	</div>
{/if}
{#if pledgeMessage}
	<div
		class="w-full bg-green-50 py-3 text-center font-ui text-ui font-medium text-green-800"
		role="status"
	>
		✓ {pledgeMessage}
	</div>
{/if}

<!-- Wordmark-only header: the giver surface has no signed-in nav. Full-bleed with the
     mark at the left edge (the export sits it well outside the content column). -->
<header class="border-b border-divider bg-page">
	<div class="flex items-center px-6 py-4 sm:px-10">
		<a
			href={resolve('/')}
			class="flex items-center gap-2.5 font-display text-display-sm font-semibold text-primary"
		>
			<span
				class="flex h-8 w-8 items-center justify-center rounded-lg bg-primary text-xl leading-none text-white"
				aria-hidden="true">Y</span
			>
			Yaadegar
		</a>
	</div>
</header>

<main class="mx-auto max-w-2xl px-4 py-10">
	{#if data.closed}
		<!-- Inactive share link. The design set does not draw this state; it is kept and
		     dressed in the tokens rather than dropped (#235). -->
		<h1 class="display-title-md font-display text-ink-heading">This list is no longer active</h1>
		<p class="mt-3 font-ui text-body text-ink-muted">
			The owner has closed it, or its event date has passed. There's nothing to reserve here right
			now.
		</p>
	{:else if data.list}
		<h1 class="display-title-md font-display text-ink-heading">{data.list.title}</h1>
		{#if data.descriptionHtml}
			<!-- data.descriptionHtml is sanitized server-side (renderNote: marked → sanitize-html
			     tight allowlist); {@html} only ever touches this pre-sanitized field (#143, ADR-0006). -->
			<!-- eslint-disable svelte/no-at-html-tags -->
			<div class="prose prose-sm mt-2 max-w-none font-ui text-body text-ink-muted">
				{@html data.descriptionHtml}
			</div>
			<!-- eslint-enable svelte/no-at-html-tags -->
		{/if}
		{#if data.list.event_date}
			<p class="mt-1 font-ui text-ui text-ink-muted">For {data.list.event_date}</p>
		{/if}

		{#if emailError}
			<p class="mt-4 rounded-card bg-red-50 p-3 font-ui text-ui text-red-600" role="alert">
				{emailError}
			</p>
		{/if}
		{#if clientEmailError}
			<p class="mt-4 rounded-card bg-red-50 p-3 font-ui text-ui text-red-600" role="alert">
				{clientEmailError}
			</p>
		{/if}
		{#if releaseError}
			<p class="mt-4 rounded-card bg-red-50 p-3 font-ui text-ui text-red-600" role="alert">
				{releaseError}
			</p>
		{/if}
		{#if withdrawError}
			<p class="mt-4 rounded-card bg-red-50 p-3 font-ui text-ui text-red-600" role="alert">
				{withdrawError}
			</p>
		{/if}

		{#if data.accountRequired}
			<!-- Registered tier (#170): the anonymous reserve path is rejected, so show an
			     account-required state instead of a reserve form. Undesigned; dressed in the
			     tokens, structure kept (#235). -->
			<div class="mt-6 rounded-card border border-line bg-gold-tint p-6">
				<p class="font-ui text-body font-medium text-ink-heading">
					This list needs an account to reserve
				</p>
				<p class="mt-1 font-ui text-ui text-ink-muted">
					The owner asked that gifts here be reserved from an account, so your reservations stay in
					one place. The owner still never sees who reserved.
				</p>
				<div class="mt-4 flex flex-wrap gap-3">
					<!-- eslint-disable svelte/no-navigation-without-resolve -- resolve() cannot express
					     the ?return_to= query; the path is resolve()'d and the value is a validated
					     local path (safeReturnTo on the server). -->
					<a
						class="inline-flex h-12 items-center justify-center rounded-card bg-primary px-6 font-ui text-ui font-medium text-white transition-colors hover:bg-primary-hover"
						href={`${resolve('/login')}?return_to=${encodeURIComponent(data.reservePath)}`}
						>Log in to reserve</a
					>
					{#if data.registrationEnabled}
						<a
							class="inline-flex h-12 items-center justify-center rounded-card border border-line bg-surface px-6 font-ui text-ui font-medium text-ink transition-colors hover:bg-surface-alt"
							href={`${resolve('/register')}?return_to=${encodeURIComponent(data.reservePath)}`}
							>Create an account</a
						>
					{/if}
					<!-- eslint-enable svelte/no-navigation-without-resolve -->
				</div>
			</div>
		{:else}
			{#if data.loggedIn}
				<!-- A signed-in visitor can reserve as their account instead, so it lands in
				     their dashboard (#170). A pill, per the export; the anonymous flow still works. -->
				<a
					class="mt-6 flex items-center gap-1 rounded-full border border-line bg-surface px-5 py-3 font-ui text-ui text-ink-muted transition-colors hover:bg-surface-alt"
					href={resolve('/(app)/reserve/[shareSlug]', { shareSlug: data.shareSlug })}
				>
					<span class="font-medium text-ink">Reserve as your account instead</span>
					<span class="text-primary" aria-hidden="true">→</span>
					<span>so it shows up in your reservations.</span>
				</a>
			{/if}
			<!-- One form drives reserve/release/pledge/withdraw: each button carries its own
			     item_id and targets its action via formaction. The optional giver identity
			     for a full reserve is entered once and applies to whichever item is reserved. -->
			<form method="post" use:enhance={guardReserve} class="mt-6">
				<!-- Your details: a tinted card, per the export (lock icon + optional identity). -->
				<fieldset class="rounded-card bg-primary-tint p-6">
					<legend
						class="flex items-center gap-2 px-1 font-ui text-body font-medium text-ink-heading"
					>
						<svg
							width="16"
							height="16"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="2"
							stroke-linecap="round"
							stroke-linejoin="round"
							class="text-primary"
							aria-hidden="true"
						>
							<rect x="3" y="11" width="18" height="11" rx="2" />
							<path d="M7 11V7a5 5 0 0 1 10 0v4" />
						</svg>
						{emailRequired ? 'Your details' : 'Your details (optional)'}
					</legend>
					<div class="mt-3 grid gap-3 sm:grid-cols-2">
						<label class="block">
							<span class="mb-1 block font-ui text-ui font-medium text-ink">Name</span>
							<input
								class="h-12 w-full rounded-card border border-line-subtle bg-surface px-3 font-ui text-body text-ink placeholder:text-ink-muted focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
								name="giver_name"
								autocomplete="name"
								placeholder="Shown to no one"
							/>
						</label>
						<label class="block">
							<span class="mb-1 block font-ui text-ui font-medium text-ink"
								>{emailRequired ? 'Email (required)' : 'Email'}</span
							>
							<input
								class="h-12 w-full rounded-card border border-line-subtle bg-surface px-3 font-ui text-body text-ink placeholder:text-ink-muted focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
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
					<!-- Privacy promise under the details card — a core product guarantee, kept. -->
					<p class="mt-3 font-ui text-ui text-ink-muted">
						{#if emailRequired}
							This list needs your email to confirm your reservation — never shown to the list owner
							or other givers. Your name is optional.
						{:else}
							Used only to remind you — never shown to the list owner or other givers.
						{/if}
					</p>
				</fieldset>

				<!-- Anti-bot captcha (ADR-0013): only on the low-trust reserve tiers, only when a
				     provider is configured. Fills the hidden captcha_token the reserve action reads. -->
				{#if data.captchaProvider}
					<CaptchaWidget provider={data.captchaProvider} siteKey={data.captchaSiteKey} />
				{/if}

				<h2 class="display-section mt-8 font-display text-ink-heading">The gifts</h2>

				<ul class="mt-4 space-y-3">
					{#each data.list.items ?? [] as item (item.id)}
						{@const reservedByYou = !!item.id && data.reservedItemIds.includes(item.id)}
						{@const pledge = item.id ? data.pledged[item.id] : undefined}
						{@const priceMajor = major(item.price)}
						{@const fundedMajor = major(item.amount_funded)}
						{@const remaining = Math.max(priceMajor - fundedMajor, 0)}
						<!-- Fail CLOSED on an absent availability: 'reserved' (chip + action agree) so an
					     unreachable-today gap can never show a taken gift as reservable and cause a
					     double-buy. The backend always populates this; the default guards the day that
					     stops being true (#235 review). -->
						{@const availability = item.availability ?? 'reserved'}
						<li
							class={`rounded-card border bg-surface p-4 ${reservedByYou ? 'border-gold ring-1 ring-gold' : 'border-line'}`}
						>
							<div class="flex items-start justify-between gap-4">
								<div class="flex min-w-0 gap-4">
									<!-- Item glyph in a tinted circle (amber when reserved by this browser). The
									     glyph-vs-photo and per-item-glyph choices are design questions, not
									     resolved here (see design-open-questions.md). -->
									<span
										class={`flex h-12 w-12 shrink-0 items-center justify-center rounded-full ${reservedByYou ? 'bg-gold-tint text-gold' : 'bg-primary-tint text-primary'}`}
										aria-hidden="true"
									>
										{@render giftGlyph()}
									</span>
									<div class="min-w-0">
										<div class="flex flex-wrap items-center gap-2">
											<span class="font-ui text-body font-medium text-ink-heading">{item.name}</span
											>
											<span
												class={`rounded-card px-2 py-0.5 font-ui text-chip ${availabilityChip[availability] ?? 'bg-surface-alt text-ink-muted'}`}
											>
												{availabilityLabel[availability] ?? 'Available'}
											</span>
										</div>
										{#if item.id && data.noteHtml[item.id]}
											<!-- data.noteHtml is sanitized server-side (marked → sanitize-html tight
											     allowlist); {@html} only ever touches this pre-sanitized field. -->
											<!-- eslint-disable svelte/no-at-html-tags -->
											<div class="prose prose-sm mt-1 max-w-none font-ui text-ui text-ink-muted">
												{@html data.noteHtml[item.id]}
											</div>
											<!-- eslint-enable svelte/no-at-html-tags -->
										{/if}
										<div
											class="mt-1 flex flex-wrap items-center gap-x-3 font-ui text-ui text-ink-muted"
										>
											{#if fmt(item.price)}<span class="font-medium text-ink"
													>{fmt(item.price)}</span
												>{/if}
											{#if item.url}
												<!-- eslint-disable svelte/no-navigation-without-resolve -- external, user-provided product URL -->
												<a
													href={item.url}
													class="text-primary-hover underline"
													rel="noreferrer"
													target="_blank">View item</a
												>
												<!-- eslint-enable svelte/no-navigation-without-resolve -->
											{/if}
										</div>

										{#if availability === 'co_buying'}
											<!-- Co-buy progress: a state + a funded amount only, never who chipped in. -->
											<div class="mt-2 max-w-xs">
												<div class="h-2 overflow-hidden rounded-full bg-surface-alt">
													<div
														class="h-full bg-green"
														style="width: {priceMajor > 0
															? Math.min(100, Math.round((fundedMajor / priceMajor) * 100))
															: 0}%"
													></div>
												</div>
												<p class="mt-1 font-ui text-chip text-ink-muted">
													{fmt(item.amount_funded)} of {fmt(item.price)} chipped in
												</p>
											</div>
										{/if}
									</div>
								</div>

								<div class="flex shrink-0 flex-col items-end gap-1 text-right">
									{#if reservedByYou}
										<p class="font-ui text-ui font-medium text-green">✓ You reserved this</p>
										<button
											class="font-ui text-ui text-primary underline hover:text-primary-hover"
											formaction="?/release"
											name="item_id"
											value={item.id}>Release</button
										>
									{:else if pledge}
										<p class="font-ui text-ui font-medium text-green">✓ You're chipping in</p>
										{#if pledge.matched && pledge.match_id}
											<a
												class="font-ui text-ui text-primary underline hover:text-primary-hover"
												href={resolve('/cobuy/[matchId]', { matchId: pledge.match_id })}
												>Confirm the group buy →</a
											>
										{:else}
											<button
												class="font-ui text-ui text-primary underline hover:text-primary-hover"
												formaction="?/withdraw"
												name="item_id"
												value={item.id}>Withdraw pledge</button
											>
										{/if}
										<button
											class="font-ui text-chip text-ink-muted underline hover:text-ink"
											formaction="?/refresh">Check for updates</button
										>
									{:else if availability === 'available'}
										<Button type="submit" formaction="?/reserve" name="item_id" value={item.id}>
											Reserve it
										</Button>
										{#if chipInAllowed(item)}
											<button
												type="button"
												class="font-ui text-ui text-primary underline hover:text-primary-hover"
												onclick={() => item.id && openChipIn(item.id)}>Chip in instead</button
											>
										{/if}
									{:else if availability === 'co_buying' && chipInAllowed(item)}
										<Button type="button" onclick={() => item.id && openChipIn(item.id)}>
											Chip in the rest
										</Button>
									{:else}
										<span class="font-ui text-ui text-ink-muted"
											>{availabilityLabel[availability] ?? 'Taken'}</span
										>
									{/if}
								</div>
							</div>

							{#if openPledge === item.id && item.price && chipInAllowed(item)}
								<!-- Inline chip-in form for this item. Amount is in the item's currency
								     (hidden field); the backend rejects a currency mismatch. -->
								<div class="mt-3 rounded-card border border-line bg-surface-alt p-4">
									<input type="hidden" name="item_id" value={item.id} />
									<input type="hidden" name="currency" value={item.price.currency} />
									<p class="font-ui text-ui font-medium text-ink">Chip in toward {item.name}</p>
									{#if remaining > 0}
										<p class="font-ui text-chip text-ink-muted">
											{remaining.toFixed(2)}
											{item.price.currency} still needed
										</p>
									{/if}
									<div class="mt-2 flex flex-wrap items-center gap-2">
										<button
											type="button"
											class="rounded-card border border-line bg-surface px-2 py-1 font-ui text-chip text-ink transition-colors hover:bg-surface-alt"
											onclick={() => setShare(priceMajor, 1 / 2)}>½</button
										>
										<button
											type="button"
											class="rounded-card border border-line bg-surface px-2 py-1 font-ui text-chip text-ink transition-colors hover:bg-surface-alt"
											onclick={() => setShare(priceMajor, 1 / 3)}>⅓</button
										>
										<button
											type="button"
											class="rounded-card border border-line bg-surface px-2 py-1 font-ui text-chip text-ink transition-colors hover:bg-surface-alt"
											onclick={() => setShare(priceMajor, 1 / 4)}>¼</button
										>
										<label class="flex items-center gap-1 font-ui text-ui text-ink">
											<span>Amount</span>
											<input
												class="h-10 w-28 rounded-card border border-line bg-surface px-3 font-ui text-body text-ink focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
												name="amount"
												type="number"
												min="0"
												step="0.01"
												bind:value={pledgeAmount}
											/>
											<span class="font-ui text-chip text-ink-muted">{item.price.currency}</span>
										</label>
									</div>
									{#if amountError}<p class="mt-1 font-ui text-chip text-red-600" role="alert">
											{amountError}
										</p>{/if}
									<label class="mt-2 block">
										<span class="mb-1 block font-ui text-ui font-medium text-ink">Your email</span>
										<input
											class="h-12 w-full rounded-card border border-line bg-surface px-3 font-ui text-body text-ink placeholder:text-ink-muted focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
											name="contact_email"
											type="email"
											autocomplete="email"
											placeholder="Revealed only if the group buy is confirmed by everyone"
										/>
									</label>
									{#if contactError}<p class="mt-1 font-ui text-chip text-red-600" role="alert">
											{contactError}
										</p>{/if}
									{#if pledgeError}<p class="mt-1 font-ui text-chip text-red-600" role="alert">
											{pledgeError}
										</p>{/if}
									<p class="mt-1 font-ui text-chip text-ink-muted">
										Your email is shown to your co-buyers only after everyone confirms — never to
										the owner.
									</p>
									<div class="mt-3 flex items-center gap-3">
										<Button type="submit" formaction="?/pledge">Pledge</Button>
										<button
											type="button"
											class="font-ui text-ui text-ink-muted underline hover:text-ink"
											onclick={() => (openPledge = null)}>Cancel</button
										>
									</div>
								</div>
							{/if}
						</li>
					{/each}
				</ul>
			</form>

			<!-- Closing privacy banner — the product's core promise, kept (#235). -->
			<p class="mt-8 rounded-card bg-surface-accent p-4 text-center font-ui text-ui text-ink-muted">
				Reserving or chipping in keeps the surprise — the owner never sees who did either.
			</p>
		{/if}
	{/if}
</main>
