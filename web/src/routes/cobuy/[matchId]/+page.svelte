<script lang="ts">
	import { enhance } from '$app/forms';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();

	// The outcome of a decision (after POST) takes precedence over the loaded state.
	const decided = $derived(form && 'view' in form ? form.view : undefined);
	const decideError = $derived(form && 'decideError' in form ? form.decideError : undefined);
	// The match as loaded (present only when data.state === 'ok').
	const loaded = $derived(data.state === 'ok' ? data.view : undefined);
	const scopedToken = $derived('t' in data ? (data.t ?? '') : '');
</script>

<svelte:head>
	<title>Confirm your group gift · Yaadegar</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<main class="mx-auto max-w-md p-6">
	{#if decided}
		<!-- After a decision. Contacts appear only when both_confirmed (matchView). -->
		{#if decided.released}
			<h1 class="text-2xl font-bold">The group buy was dissolved</h1>
			<p class="mt-3 text-gray-600">
				Someone declined, so the match is off and your pledge has been released. You can chip in
				again from the list if you like.
			</p>
		{:else if decided.state === 'both_confirmed'}
			<h1 class="text-2xl font-bold">You're all set 🎁</h1>
			<p class="mt-3 text-gray-600">
				Everyone confirmed. Coordinate the purchase with your {decided.contacts.length} co-buyer{decided
					.contacts.length === 1
					? ''
					: 's'}:
			</p>
			<ul class="mt-2 list-inside list-disc text-sm">
				{#each decided.contacts as contact (contact)}
					<li>{contact}</li>
				{/each}
			</ul>
		{:else}
			<h1 class="text-2xl font-bold">Thanks — you've confirmed</h1>
			<p class="mt-3 text-gray-600">
				Waiting for the other giver{decided.participants > 2 ? 's' : ''} to confirm. Everyone gets an
				email with the group's contacts once the last person confirms.
			</p>
		{/if}
	{:else if data.state === 'resolved'}
		<h1 class="text-2xl font-bold">This group buy is already confirmed</h1>
		<p class="mt-3 text-gray-600">
			Nothing more to do here — check your email for everyone's contacts so you can coordinate the
			purchase.
		</p>
	{:else if data.state === 'expired'}
		<h1 class="text-2xl font-bold">This link has expired</h1>
		<p class="mt-3 text-gray-600">
			The confirmation window passed. If the item is still available you can chip in again from the
			list.
		</p>
	{:else if data.state === 'error'}
		<h1 class="text-2xl font-bold">Something went wrong</h1>
		<p class="mt-3 text-gray-600">Please try again in a moment.</p>
	{:else if data.state === 'invalid' || !loaded}
		<h1 class="text-2xl font-bold">This link isn't valid</h1>
		<p class="mt-3 text-gray-600">
			It may have already been used, or needs the device you pledged from. Head back to the list to
			check your pledge.
		</p>
	{:else if loaded.state === 'both_confirmed'}
		<h1 class="text-2xl font-bold">This group buy is confirmed 🎁</h1>
		<p class="mt-3 text-gray-600">Coordinate the purchase with your co-buyers:</p>
		<ul class="mt-2 list-inside list-disc text-sm">
			{#each loaded.contacts as contact (contact)}
				<li>{contact}</li>
			{/each}
		</ul>
	{:else if loaded.state === 'expired'}
		<h1 class="text-2xl font-bold">This group buy expired</h1>
		<p class="mt-3 text-gray-600">
			No one confirmed in time, so the match was called off and your pledge released. You can chip
			in again from the list if you like.
		</p>
	{:else if loaded.released}
		<h1 class="text-2xl font-bold">This group buy was dissolved</h1>
		<p class="mt-3 text-gray-600">Your pledge has been released.</p>
	{:else}
		<!-- proposed → offer confirm/decline -->
		<h1 class="text-2xl font-bold">A group gift is ready</h1>
		<p class="mt-3 text-gray-600">
			Enough people have chipped in to cover this item. Confirm to go ahead with the group buy —
			everyone's contacts are shared only after all {loaded.participants} of you confirm. Declining releases
			your pledge.
		</p>
		{#if decideError}
			<p class="mt-3 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">{decideError}</p>
		{/if}
		<form method="post" action="?/decide" use:enhance class="mt-5 flex gap-2">
			<input type="hidden" name="t" value={scopedToken} />
			<button class="rounded bg-black px-4 py-2 text-sm text-white" name="decision" value="confirm"
				>Confirm the group buy</button
			>
			<button class="rounded border px-4 py-2 text-sm" name="decision" value="decline"
				>Decline</button
			>
		</form>
	{/if}
</main>
