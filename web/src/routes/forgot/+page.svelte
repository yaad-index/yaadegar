<script lang="ts">
	import { enhance } from '$app/forms';
	import { resolve } from '$app/paths';
	import AuthPanel from '$lib/components/AuthPanel.svelte';
	import Field from '$lib/components/Field.svelte';
	import Button from '$lib/components/Button.svelte';
	import type { ActionData } from './$types';

	let { form }: { form: ActionData } = $props();
</script>

<svelte:head><title>Forgot password · Yaadegar</title></svelte:head>

<AuthPanel heading="Forgot your password?">
	{#if form?.sent}
		<p class="rounded-card bg-green-50 px-3 py-2 font-ui text-ui text-green-700" role="status">
			If that account exists, we've emailed a link to reset its password. The link expires soon and
			can be used once.
		</p>
	{:else}
		<p class="mb-4 font-ui text-body text-ink-muted">
			Enter your username or email and we'll send a reset link.
		</p>
		{#if form?.error}
			<p class="mb-4 rounded-card bg-red-50 px-3 py-2 font-ui text-ui text-red-700" role="alert">
				{form.error}
			</p>
		{/if}
		<form method="post" use:enhance class="space-y-4">
			<Field label="Username or email" name="identifier" autocomplete="username" required />
			<Button type="submit" full>Send reset link</Button>
		</form>
	{/if}

	{#snippet links()}
		<p>
			<a class="text-primary-hover hover:underline" href={resolve('/login')}>Back to sign in</a>
		</p>
	{/snippet}
</AuthPanel>
