<script lang="ts">
	import { enhance } from '$app/forms';
	import { resolve } from '$app/paths';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();
</script>

<svelte:head><title>Reset password · Yaadegar</title></svelte:head>

<main class="mx-auto max-w-sm p-8">
	<h1 class="text-2xl font-bold">Choose a new password</h1>

	{#if !data.token}
		<p class="mt-4 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">
			This reset link is missing its token. Request a new one from the
			<a class="underline" href={resolve('/forgot')}>forgot-password page</a>.
		</p>
	{:else}
		{#if form?.error}
			<p class="mt-3 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">{form.error}</p>
		{/if}
		<form method="post" use:enhance class="mt-4 space-y-3">
			<input type="hidden" name="token" value={data.token} />
			<label class="block">
				<span class="text-sm font-medium">New password</span>
				<input
					class="mt-1 w-full rounded border p-2"
					type="password"
					name="new_password"
					autocomplete="new-password"
					minlength="8"
					required
				/>
			</label>
			<label class="block">
				<span class="text-sm font-medium">Confirm new password</span>
				<input
					class="mt-1 w-full rounded border p-2"
					type="password"
					name="confirm_password"
					autocomplete="new-password"
					minlength="8"
					required
				/>
			</label>
			<button class="w-full rounded bg-black px-3 py-2 text-white" type="submit">
				Set new password
			</button>
		</form>
	{/if}
</main>
