<script lang="ts">
	import { enhance } from '$app/forms';
	import { resolve } from '$app/paths';
	import type { ActionData } from './$types';

	let { form }: { form: ActionData } = $props();
</script>

<svelte:head><title>Create an account · Yaadegar</title></svelte:head>

<main class="mx-auto max-w-sm p-8">
	<h1 class="text-2xl font-bold">Create an account</h1>

	{#if form?.sent}
		<p class="mt-4 rounded bg-green-50 p-2 text-sm text-green-700" role="status">
			Check your email to verify your account. The link expires soon and can be used once.
		</p>
	{:else if form?.disabled}
		<p class="mt-4 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">
			Registration isn't enabled on this instance.
		</p>
	{:else}
		<p class="mt-2 text-sm text-gray-600">
			Enter your email and choose a password. We'll send a link to verify your email.
		</p>
		{#if form?.error}
			<p class="mt-3 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">{form.error}</p>
		{/if}
		<form method="post" use:enhance class="mt-4 space-y-3">
			<label class="block">
				<span class="text-sm font-medium">Email</span>
				<input
					class="mt-1 w-full rounded border p-2"
					type="email"
					name="email"
					autocomplete="email"
					required
				/>
			</label>
			<label class="block">
				<span class="text-sm font-medium">Password</span>
				<input
					class="mt-1 w-full rounded border p-2"
					type="password"
					name="password"
					autocomplete="new-password"
					minlength="8"
					required
				/>
			</label>
			<label class="block">
				<span class="text-sm font-medium">Confirm password</span>
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
				Create account
			</button>
		</form>
	{/if}

	<p class="mt-4 text-sm">
		<a class="text-gray-600 underline hover:text-gray-900" href={resolve('/login')}
			>Back to sign in</a
		>
	</p>
</main>
