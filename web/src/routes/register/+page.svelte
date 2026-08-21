<script lang="ts">
	import { enhance } from '$app/forms';
	import { resolve } from '$app/paths';
	import AuthPanel from '$lib/components/AuthPanel.svelte';
	import Field from '$lib/components/Field.svelte';
	import PasswordField from '$lib/components/PasswordField.svelte';
	import Button from '$lib/components/Button.svelte';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();
</script>

<svelte:head><title>Create an account · Yaadegar</title></svelte:head>

<AuthPanel heading="Create an account">
	{#if form?.sent}
		<p class="rounded-card bg-green-50 px-3 py-2 font-ui text-ui text-green-700" role="status">
			Check your email to verify your account. The link expires soon and can be used once.
		</p>
	{:else if form?.disabled || !data.registrationEnabled}
		<!-- Shown both up front, when the loader reports the instance policy disables
		     self-registration (#253), and as the action's 403 fallback — one message so a
		     visitor who reaches /register by a link, bookmark, or typed path is told the
		     same thing without first filling a form that can only be refused. -->
		<p class="rounded-card bg-red-50 px-3 py-2 font-ui text-ui text-red-700" role="alert">
			Registration isn't enabled on this instance.
		</p>
	{:else}
		<p class="mb-4 font-ui text-body text-ink-muted">
			Enter your email and choose a password. We'll send a link to verify your email.
		</p>
		{#if form?.error}
			<p class="mb-4 rounded-card bg-red-50 px-3 py-2 font-ui text-ui text-red-700" role="alert">
				{form.error}
			</p>
		{/if}
		<form
			method="post"
			action={data.returnTo ? `?return_to=${encodeURIComponent(data.returnTo)}` : undefined}
			use:enhance
			class="space-y-4"
		>
			<Field label="Email" type="email" name="email" autocomplete="email" required />
			<PasswordField
				label="Password"
				name="password"
				autocomplete="new-password"
				minlength={8}
				required
			/>
			<PasswordField
				label="Confirm password"
				name="confirm_password"
				autocomplete="new-password"
				minlength={8}
				required
			/>
			<Button type="submit" full>Create account</Button>
		</form>
	{/if}

	{#snippet links()}
		<p>
			<!-- eslint-disable svelte/no-navigation-without-resolve -- resolve() cannot express the
			?return_to= query; the path is resolve()'d and the value is a validated local path. -->
			<a
				class="text-primary-hover hover:underline"
				href={data.returnTo
					? `${resolve('/login')}?return_to=${encodeURIComponent(data.returnTo)}`
					: resolve('/login')}
			>
				Back to log in
			</a>
			<!-- eslint-enable svelte/no-navigation-without-resolve -->
		</p>
	{/snippet}
</AuthPanel>
