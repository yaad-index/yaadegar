<script lang="ts">
	import { enhance } from '$app/forms';
	import { txtRecordName } from '$lib/domains';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();
	// After a toggle save the action returns fresh settings; otherwise use the load.
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
	<form method="post" action="?/toggle" use:enhance class="mt-2">
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

<section class="mt-8">
	<h2 class="font-medium">Custom domains</h2>
	<p class="mt-1 text-sm text-gray-600">
		Serve your lists from your own hostname. Add it, publish the two DNS records shown, then verify.
	</p>

	{#if form?.domainError}
		<p class="mt-3 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">{form.domainError}</p>
	{/if}
	{#if form?.addedHostname}
		<p class="mt-3 rounded bg-green-50 p-2 text-sm text-green-700" role="status">
			Added {form.addedHostname}. Publish the DNS records below, then verify.
		</p>
	{/if}

	<form method="post" action="?/addDomain" use:enhance class="mt-3 flex gap-2">
		<input
			class="w-full rounded border p-2"
			name="hostname"
			placeholder="gifts.your-domain.example"
			autocomplete="off"
		/>
		<button class="shrink-0 rounded bg-black px-3 py-2 text-white" type="submit">Add domain</button>
	</form>

	<ul class="mt-4 space-y-4">
		{#each data.domains as domain (domain.id)}
			<li class="rounded border p-3">
				<div class="flex items-center justify-between">
					<span class="font-medium">{domain.hostname}</span>
					{#if domain.verified}
						<span class="rounded bg-green-100 px-2 py-0.5 text-xs text-green-800">Verified</span>
					{:else}
						<span class="rounded bg-yellow-100 px-2 py-0.5 text-xs text-yellow-800"
							>Not verified</span
						>
					{/if}
				</div>

				{#if !domain.verified}
					<div class="mt-2 space-y-2 text-sm">
						<p class="text-gray-600">Publish these DNS records, then verify:</p>
						<dl class="space-y-1 rounded bg-gray-50 p-2 font-mono text-xs">
							<div>
								<dt class="text-gray-500">CNAME · {domain.hostname}</dt>
								<dd class="break-all">{domain.cname_target}</dd>
							</div>
							<div>
								<dt class="text-gray-500">TXT · {txtRecordName(domain.hostname ?? '')}</dt>
								<dd class="break-all">{domain.verification_token}</dd>
							</div>
						</dl>
						{#if form?.verifiedId === domain.id && form?.nowVerified === false}
							<p class="text-yellow-700">
								Not verified yet — DNS changes can take a while to propagate. Try again shortly.
							</p>
						{/if}
					</div>
				{/if}

				<div class="mt-3 flex gap-2">
					{#if !domain.verified}
						<form method="post" action="?/verifyDomain" use:enhance>
							<input type="hidden" name="id" value={domain.id} />
							<button class="rounded border px-3 py-1 text-sm" type="submit">Verify</button>
						</form>
					{/if}
					<form method="post" action="?/removeDomain" use:enhance>
						<input type="hidden" name="id" value={domain.id} />
						<button class="rounded border px-3 py-1 text-sm text-red-700" type="submit"
							>Remove</button
						>
					</form>
				</div>
			</li>
		{:else}
			<li class="text-sm text-gray-500">No custom domains yet.</li>
		{/each}
	</ul>
</section>
