<script lang="ts">
	import { enhance } from '$app/forms';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();

	// Outcome of a confirm/decline (present only after the POST).
	const view = $derived(form && 'view' in form ? form.view : undefined);
	const decideError = $derived(form && 'decideError' in form ? form.decideError : undefined);
</script>

<svelte:head>
	<title>Confirm your group gift · Yaadegar</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<main class="mx-auto max-w-md p-6">
	{#if view}
		<!-- After a decision. Contacts are present only when the match is both_confirmed
		     (matchView enforces it); nothing leaks for proposed/declined. -->
		{#if view.released}
			<h1 class="text-2xl font-bold">The group buy was dissolved</h1>
			<p class="mt-3 text-gray-600">
				Someone declined, so the match is off and your pledge has been released. You can chip in
				again from the list if you like.
			</p>
		{:else if view.state === 'both_confirmed'}
			<h1 class="text-2xl font-bold">You're all set 🎁</h1>
			<p class="mt-3 text-gray-600">
				Everyone confirmed. Coordinate the purchase with your {view.contacts.length} co-buyer{view
					.contacts.length === 1
					? ''
					: 's'}:
			</p>
			<ul class="mt-2 list-inside list-disc text-sm">
				{#each view.contacts as contact (contact)}
					<li>{contact}</li>
				{/each}
			</ul>
		{:else}
			<h1 class="text-2xl font-bold">Thanks — you've confirmed</h1>
			<p class="mt-3 text-gray-600">
				Waiting for the other giver{view.participants > 2 ? 's' : ''} to confirm. Everyone gets an email
				with the group's contacts once the last person confirms.
			</p>
		{/if}
	{:else if data.state === 'invalid'}
		<h1 class="text-2xl font-bold">This link isn't valid</h1>
		<p class="mt-3 text-gray-600">
			It may have already been used or was mistyped. Head back to the list to check your pledge.
		</p>
	{:else if data.state === 'not_here'}
		<h1 class="text-2xl font-bold">Open this on the device you pledged from</h1>
		<p class="mt-3 text-gray-600">
			For now, confirming a group buy works only in the browser where you made the pledge (that's
			where your private key is kept). Open the list there to confirm.
		</p>
	{:else if data.state === 'error'}
		<h1 class="text-2xl font-bold">Something went wrong</h1>
		<p class="mt-3 text-gray-600">Please try again in a moment.</p>
	{:else if data.status === 'matched'}
		<h1 class="text-2xl font-bold">A group gift is ready</h1>
		<p class="mt-3 text-gray-600">
			Enough people have chipped in to cover this item. Confirm to go ahead with the group buy —
			everyone's contacts are shared only after all of you confirm. Declining releases your pledge.
		</p>
		{#if decideError}
			<p class="mt-3 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">{decideError}</p>
		{/if}
		<form method="post" action="?/decide" use:enhance class="mt-5 flex gap-2">
			<input type="hidden" name="contribution_id" value={data.contributionId} />
			<button class="rounded bg-black px-4 py-2 text-sm text-white" name="decision" value="confirm"
				>Confirm the group buy</button
			>
			<button class="rounded border px-4 py-2 text-sm" name="decision" value="decline"
				>Decline</button
			>
		</form>
	{:else if data.status === 'confirmed'}
		<h1 class="text-2xl font-bold">You've already confirmed</h1>
		<p class="mt-3 text-gray-600">
			Waiting on the other givers. You'll get an email with everyone's contacts once the last person
			confirms.
		</p>
	{:else if data.status === 'declined'}
		<h1 class="text-2xl font-bold">This group buy was dissolved</h1>
		<p class="mt-3 text-gray-600">Your pledge has been released.</p>
	{:else if data.status === 'withdrawn'}
		<h1 class="text-2xl font-bold">Pledge withdrawn</h1>
		<p class="mt-3 text-gray-600">You've already withdrawn this pledge.</p>
	{:else}
		<h1 class="text-2xl font-bold">No match yet</h1>
		<p class="mt-3 text-gray-600">
			Your pledge is in. We'll email you when enough people have chipped in to confirm the group
			buy.
		</p>
	{/if}
</main>
