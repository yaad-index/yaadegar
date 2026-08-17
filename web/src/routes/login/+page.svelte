<script lang="ts">
	import { superForm } from 'sveltekit-superforms';
	import { resolve } from '$app/paths';
	import AuthPanel from '$lib/components/AuthPanel.svelte';
	import Field from '$lib/components/Field.svelte';
	import PasswordField from '$lib/components/PasswordField.svelte';
	import Button from '$lib/components/Button.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	// superForm captures the initial form once and owns its reactivity thereafter.
	// svelte-ignore state_referenced_locally
	const { form, errors, message, enhance, submitting } = superForm(data.form);
</script>

<svelte:head><title>Sign in · Yaadegar</title></svelte:head>

<AuthPanel heading="Sign in">
	{#if data.oauthError}
		<!-- Rendered as text: Svelte escapes interpolation, so a crafted oauth_error
		     value can never inject markup (reflected-XSS guard). -->
		<p class="mb-4 rounded-card bg-red-50 px-3 py-2 font-ui text-ui text-red-700" role="alert">
			{data.oauthError}
		</p>
	{/if}
	{#if $message}
		<p class="mb-4 rounded-card bg-red-50 px-3 py-2 font-ui text-ui text-red-700" role="alert">
			{$message}
		</p>
	{/if}

	{#if data.methods.google}
		<!-- eslint-disable svelte/no-navigation-without-resolve -- targets the backend OAuth
		passthrough endpoint (a server route, not a page), so resolve() does not apply; must
		be a full top-level navigation so the provider redirect works. -->
		<a
			href={`/api/v1/auth/oauth/google/start?tenant=${encodeURIComponent(data.host)}&return_to=${encodeURIComponent(data.returnTo || '/')}`}
			class="inline-flex h-12 w-full items-center justify-center rounded-card border border-line bg-surface px-6 font-ui text-ui font-medium text-ink transition-colors hover:bg-surface-alt focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
			data-testid="google-signin"
		>
			Sign in with Google
		</a>
		<!-- eslint-enable svelte/no-navigation-without-resolve -->
	{/if}

	{#if data.methods.google && data.methods.password}
		<div class="my-4 flex items-center gap-3 font-ui text-ui text-ink-muted">
			<span class="h-px flex-1 bg-line"></span>or<span class="h-px flex-1 bg-line"></span>
		</div>
	{/if}

	{#if data.methods.password}
		<form
			method="post"
			action={data.returnTo ? `?return_to=${encodeURIComponent(data.returnTo)}` : undefined}
			use:enhance
			class="space-y-4"
		>
			<Field
				label="Username"
				name="username"
				autocomplete="username"
				placeholder="Enter your username"
				bind:value={$form.username}
				error={$errors.username?.[0]}
			/>
			<PasswordField
				label="Password"
				name="password"
				autocomplete="current-password"
				bind:value={$form.password}
				error={$errors.password?.[0]}
			/>
			<Button type="submit" full disabled={$submitting}>Sign in</Button>
		</form>
	{/if}

	{#snippet links()}
		<p>
			<a class="text-primary-hover hover:underline" href={resolve('/forgot')}>
				Forgot your password?
			</a>
		</p>
		{#if data.methods.registration_enabled}
			<p>
				Don't have an account?
				<!-- eslint-disable svelte/no-navigation-without-resolve -- resolve() cannot express the
				?return_to= query; the path is resolve()'d and the value is a validated local path. -->
				<a
					class="text-primary-hover hover:underline"
					href={data.returnTo
						? `${resolve('/register')}?return_to=${encodeURIComponent(data.returnTo)}`
						: resolve('/register')}
				>
					Sign up
				</a>
				<!-- eslint-enable svelte/no-navigation-without-resolve -->
			</p>
		{/if}
	{/snippet}
</AuthPanel>
