<script lang="ts">
	import { enhance } from '$app/forms';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();
	// After a save the action returns the fresh settings; otherwise use the load.
	const settings = $derived(form?.settings ?? data.settings);
</script>

<svelte:head><title>Settings · Yaadegar</title></svelte:head>

<h1 class="text-2xl font-bold">Settings</h1>

{#if form?.saved}
	<p class="mt-3 rounded bg-green-50 p-2 text-sm text-green-700" role="status">Saved.</p>
{/if}
{#if form?.error}
	<p class="mt-3 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">{form.error}</p>
{/if}

<section class="mt-6">
	<h2 class="font-medium">Owner login</h2>
	<form method="post" use:enhance class="mt-2">
		<label class="flex items-center gap-2">
			<input
				type="checkbox"
				name="oauth_google_enabled"
				checked={settings.oauth_google_enabled}
				disabled={!settings.google_client_configured}
				onchange={(e) => e.currentTarget.form?.requestSubmit()}
			/>
			<span>Allow owners to sign in with Google</span>
		</label>
		{#if !settings.google_client_configured}
			<p class="mt-1 text-xs text-gray-500">
				Google login isn't configured on this instance, so this toggle has no effect yet.
			</p>
		{/if}
		<noscript
			><button class="mt-2 rounded border px-3 py-1 text-sm" type="submit">Save</button></noscript
		>
	</form>
</section>
