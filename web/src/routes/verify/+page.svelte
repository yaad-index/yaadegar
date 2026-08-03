<script lang="ts">
	import { enhance } from '$app/forms';
	import { resolve } from '$app/paths';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();
</script>

<svelte:head><title>Verify your email · Yaadegar</title></svelte:head>

<main class="mx-auto max-w-sm p-8">
	<h1 class="text-2xl font-bold">Verify your email</h1>

	{#if !data.token}
		<p class="mt-4 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">
			This verification link is missing its token. Request a new one from the
			<a class="underline" href={resolve('/register')}>sign-up page</a>.
		</p>
	{:else}
		{#if form?.error}
			<p class="mt-3 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">{form.error}</p>
		{/if}
		<p class="mt-2 text-sm text-gray-600">
			Click below to verify your email and finish creating your account.
		</p>
		<form method="post" use:enhance class="mt-4">
			<input type="hidden" name="token" value={data.token} />
			<button class="w-full rounded bg-black px-3 py-2 text-white" type="submit">
				Verify email
			</button>
		</form>
	{/if}
</main>
