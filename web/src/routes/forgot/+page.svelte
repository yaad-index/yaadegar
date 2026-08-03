<script lang="ts">
	import { enhance } from '$app/forms';
	import { resolve } from '$app/paths';
	import type { ActionData } from './$types';

	let { form }: { form: ActionData } = $props();
</script>

<svelte:head><title>Forgot password · Yaadegar</title></svelte:head>

<main class="mx-auto max-w-sm p-8">
	<h1 class="text-2xl font-bold">Forgot your password?</h1>

	{#if form?.sent}
		<p class="mt-4 rounded bg-green-50 p-2 text-sm text-green-700" role="status">
			If that account exists, we've emailed a link to reset its password. The link expires soon and
			can be used once.
		</p>
	{:else}
		<p class="mt-2 text-sm text-gray-600">
			Enter your username or email and we'll send a reset link.
		</p>
		{#if form?.error}
			<p class="mt-3 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">{form.error}</p>
		{/if}
		<form method="post" use:enhance class="mt-4 space-y-3">
			<label class="block">
				<span class="text-sm font-medium">Username or email</span>
				<input
					class="mt-1 w-full rounded border p-2"
					name="identifier"
					autocomplete="username"
					required
				/>
			</label>
			<button class="w-full rounded bg-black px-3 py-2 text-white" type="submit">
				Send reset link
			</button>
		</form>
	{/if}

	<p class="mt-4 text-sm">
		<a class="text-gray-600 underline hover:text-gray-900" href={resolve('/login')}
			>Back to sign in</a
		>
	</p>
</main>
