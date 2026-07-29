<script lang="ts">
	import { enhance } from '$app/forms';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();

	// Discriminate the action outcomes. Before any POST, `form` is null and we show
	// the confirm button (as long as the link carried a token).
	const confirmed = $derived(form && 'state' in form && form.state === 'confirmed');
	const expired = $derived(form && 'state' in form && form.state === 'expired');
	const invalid = $derived(form && 'state' in form && form.state === 'invalid');
	const canRelease = $derived(!!(form && 'canRelease' in form && form.canRelease));
	const reservationId = $derived(form && 'reservationId' in form ? form.reservationId : undefined);
	const released = $derived(!!(form && 'released' in form && form.released));
	const releaseError = $derived(form && 'releaseError' in form ? form.releaseError : undefined);
</script>

<svelte:head>
	<title>Confirm your reservation · Yaadegar</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<main class="mx-auto max-w-md p-6">
	{#if released}
		<h1 class="text-2xl font-bold">Reservation released</h1>
		<p class="mt-3 text-gray-600">The item is available for someone else to reserve.</p>
	{:else if confirmed}
		<h1 class="text-2xl font-bold">Reservation confirmed 🎁</h1>
		<p class="mt-3 text-gray-600">
			You're all set — the item is reserved for you, and the owner never sees who reserved it.
		</p>
		{#if canRelease && reservationId}
			<p class="mt-4 text-sm text-gray-500">Changed your mind?</p>
			<form method="post" action="?/release" use:enhance class="mt-1">
				<input type="hidden" name="reservation_id" value={reservationId} />
				<button class="text-sm text-gray-600 underline">Release this reservation</button>
			</form>
		{/if}
		{#if releaseError}
			<p class="mt-3 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">{releaseError}</p>
		{/if}
	{:else if expired}
		<h1 class="text-2xl font-bold">This link has expired</h1>
		<p class="mt-3 text-gray-600">
			The confirmation window passed, so the item was released for others. You can reserve it again
			from the list if it's still available.
		</p>
	{:else if invalid || !data.token}
		<h1 class="text-2xl font-bold">This link isn't valid</h1>
		<p class="mt-3 text-gray-600">
			It may have already been used or was mistyped. If you still want the item, reserve it again
			from the list.
		</p>
	{:else}
		<h1 class="text-2xl font-bold">Confirm your reservation</h1>
		<p class="mt-3 text-gray-600">
			Click below to confirm and hold the item. We ask for a click so an email scanner can't confirm
			it for you.
		</p>
		<!-- Confirm only on POST: the token rides a hidden field, never a GET, so
		     link-prefetch/scanners can't consume the one-time confirm. -->
		<form method="post" action="?/confirm" use:enhance class="mt-5">
			<input type="hidden" name="token" value={data.token} />
			<button class="rounded bg-black px-4 py-2 text-sm text-white">Confirm reservation</button>
		</form>
	{/if}
</main>
