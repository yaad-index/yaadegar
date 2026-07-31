<script lang="ts">
	import { superForm } from 'sveltekit-superforms';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	// superForm captures the initial form once and owns its reactivity thereafter.
	// svelte-ignore state_referenced_locally
	const { form, errors, message, enhance, submitting } = superForm(data.form);
</script>

<svelte:head><title>Log in · Yaadegar</title></svelte:head>

<main class="mx-auto max-w-sm p-8">
	<h1 class="text-2xl font-bold">Log in</h1>
	{#if data.oauthError}
		<!-- Rendered as text: Svelte escapes interpolation, so a crafted oauth_error
		     value can never inject markup (reflected-XSS guard). -->
		<p class="mt-3 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">{data.oauthError}</p>
	{/if}
	{#if $message}
		<p class="mt-3 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">{$message}</p>
	{/if}

	{#if data.methods.google}
		<!-- eslint-disable svelte/no-navigation-without-resolve -- targets the backend OAuth
		passthrough endpoint (a server route, not a page), so resolve() does not apply; must
		be a full top-level navigation so the provider redirect works. -->
		<a
			href={`/api/v1/auth/oauth/google/start?tenant=${encodeURIComponent(data.host)}&return_to=%2F`}
			class="mt-6 flex w-full items-center justify-center rounded border px-3 py-2 font-medium hover:bg-gray-50"
			data-testid="google-signin"
		>
			Sign in with Google
		</a>
		<!-- eslint-enable svelte/no-navigation-without-resolve -->
	{/if}

	{#if data.methods.google && data.methods.password}
		<div class="my-4 flex items-center gap-3 text-xs text-gray-400">
			<span class="h-px flex-1 bg-gray-200"></span>or<span class="h-px flex-1 bg-gray-200"></span>
		</div>
	{/if}

	{#if data.methods.password}
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
				disabled={$submitting}>Log in</button
			>
		</form>
	{/if}
</main>
