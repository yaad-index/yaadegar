<script lang="ts">
	import { superForm } from 'sveltekit-superforms';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	// svelte-ignore state_referenced_locally
	const { form, errors, message, enhance, submitting } = superForm(data.form);
</script>

<svelte:head><title>Admin sign in · Yaadegar</title></svelte:head>

<main class="mx-auto max-w-sm p-8">
	<h1 class="text-2xl font-bold">Admin sign in</h1>
	<p class="mt-1 text-sm text-gray-600">Instance administration.</p>
	{#if $message}
		<p class="mt-3 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">{$message}</p>
	{/if}
	<form method="post" use:enhance class="mt-6 space-y-4">
		<label class="block">
			<span class="text-sm font-medium">Username</span>
			<input
				class="mt-1 w-full rounded border p-2"
				name="username"
				autocomplete="username"
				bind:value={$form.username}
			/>
			{#if $errors.username}<span class="text-xs text-red-600">{$errors.username}</span>{/if}
		</label>
		<label class="block">
			<span class="text-sm font-medium">Password</span>
			<input
				class="mt-1 w-full rounded border p-2"
				type="password"
				name="password"
				autocomplete="current-password"
				bind:value={$form.password}
			/>
			{#if $errors.password}<span class="text-xs text-red-600">{$errors.password}</span>{/if}
		</label>
		<button
			class="w-full rounded bg-black px-3 py-2 text-white disabled:opacity-50"
			disabled={$submitting}>Sign in</button
		>
	</form>
</main>
