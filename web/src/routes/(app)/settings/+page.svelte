<script lang="ts">
	import { enhance } from '$app/forms';
	import { resolve } from '$app/paths';
	import DomainDnsRecords from '$lib/components/DomainDnsRecords.svelte';
	import Field from '$lib/components/Field.svelte';
	import Button from '$lib/components/Button.svelte';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();
	// After a toggle save the action returns fresh settings; otherwise use the load.
	const settings = $derived(form?.settings ?? data.settings);
</script>

<svelte:head><title>Settings · Yaadegar</title></svelte:head>

<a
	href={resolve('/(app)/lists')}
	class="font-ui text-ui text-primary transition-colors hover:text-primary-hover"
	>← Back to dashboard</a
>
<!-- "Settings" is the 32px display rung, shared with the list-name title; the page
     title role is size-per-role at 700 weight (#234, sizes pinned in the issue). -->
<h1 class="display-list-title mt-1 font-display text-ink-heading">Settings</h1>

<div class="mt-8 space-y-6">
	<section class="rounded-card border border-line bg-surface p-6">
		<h2 class="font-display text-title text-ink-heading">Owner login</h2>
		<p class="mt-1 font-ui text-ui text-ink-muted">
			Manage authentication preferences for your account access.
		</p>

		{#if form?.saved}
			<p class="mt-4 rounded-card bg-green-tint p-3 font-ui text-ui text-green" role="status">
				Saved.
			</p>
		{/if}
		{#if form?.error}
			<p class="mt-4 rounded-card bg-red-50 p-3 font-ui text-ui text-red-600" role="alert">
				{form.error}
			</p>
		{/if}

		<form method="post" action="?/toggle" use:enhance class="mt-4">
			<label class="flex items-center gap-2.5 font-ui text-body text-ink">
				<input
					type="checkbox"
					name="oauth_google_enabled"
					class="h-4 w-4 accent-primary"
					checked={settings.oauth_google_enabled}
					disabled={!settings.google_client_configured}
					onchange={(e) => e.currentTarget.form?.requestSubmit()}
				/>
				<span>Allow owners to sign in with Google</span>
			</label>
			{#if !settings.google_client_configured}
				<p class="mt-2 font-ui text-ui text-ink-muted">
					Google login isn't configured on this instance, so this toggle has no effect yet.
				</p>
			{/if}
			<noscript>
				<div class="mt-3"><Button type="submit" variant="secondary">Save</Button></div>
			</noscript>
		</form>
	</section>

	<section class="rounded-card border border-line bg-surface p-6">
		<h2 class="font-display text-title text-ink-heading">Display name</h2>
		<p class="mt-1 font-ui text-ui text-ink-muted">
			The name shown across your account. Leave it blank to fall back to your email.
		</p>

		{#if form?.nameSaved}
			<p class="mt-4 rounded-card bg-green-tint p-3 font-ui text-ui text-green" role="status">
				Name saved.
			</p>
		{/if}
		{#if form?.nameError}
			<p class="mt-4 rounded-card bg-red-50 p-3 font-ui text-ui text-red-600" role="alert">
				{form.nameError}
			</p>
		{/if}

		<form method="post" action="?/updateName" use:enhance class="mt-4">
			<div class="flex flex-wrap items-end gap-3">
				<div class="min-w-0 flex-1">
					<Field
						label="Display name"
						name="name"
						maxlength={200}
						autocomplete="name"
						value={data.user.name ?? ''}
					/>
				</div>
				<Button type="submit">Save name</Button>
			</div>
		</form>
	</section>

	<section class="rounded-card border border-line bg-surface p-6">
		<h2 class="font-display text-title text-ink-heading">Password</h2>
		<p class="mt-1 font-ui text-ui text-ink-muted">
			Change your password. You'll stay signed in here; any other devices signed in with the old
			password are signed out.
		</p>

		{#if form?.passwordChanged}
			<p class="mt-4 rounded-card bg-green-tint p-3 font-ui text-ui text-green" role="status">
				Password changed.
			</p>
		{/if}
		{#if form?.passwordError}
			<p class="mt-4 rounded-card bg-red-50 p-3 font-ui text-ui text-red-600" role="alert">
				{form.passwordError}
			</p>
		{/if}

		<!-- Plain styled password inputs, matching the export (which shows no show/hide
		     affordance). The app has a PasswordField with a visibility toggle (#201);
		     using it here would add behaviour the design doesn't draw — flagged for the
		     designer rather than introduced under a visual migration (#198/#234). -->
		<form method="post" action="?/changePassword" use:enhance class="mt-4 space-y-4">
			<Field
				label="Current password"
				type="password"
				name="current_password"
				autocomplete="current-password"
				required
			/>
			<Field
				label="New password"
				type="password"
				name="new_password"
				autocomplete="new-password"
				minlength={8}
				required
			/>
			<Field
				label="Confirm new password"
				type="password"
				name="confirm_password"
				autocomplete="new-password"
				minlength={8}
				required
			/>
			<Button type="submit">Change password</Button>
		</form>
	</section>

	<section class="rounded-card border border-line bg-surface p-6">
		<h2 class="font-display text-title text-ink-heading">Custom domains</h2>
		<p class="mt-1 font-ui text-ui text-ink-muted">
			Serve your lists from your own hostname. Add it, publish the two DNS records shown, then
			verify.
		</p>

		{#if form?.domainError}
			<p class="mt-4 rounded-card bg-red-50 p-3 font-ui text-ui text-red-600" role="alert">
				{form.domainError}
			</p>
		{/if}
		{#if form?.addedHostname}
			<p class="mt-4 rounded-card bg-green-tint p-3 font-ui text-ui text-green" role="status">
				Added {form.addedHostname}. Publish the DNS records below, then verify.
			</p>
		{/if}

		<form
			method="post"
			action="?/addDomain"
			use:enhance
			class="mt-4 flex flex-wrap items-end gap-3"
		>
			<div class="min-w-0 flex-1">
				<Field
					label="Domain"
					name="hostname"
					placeholder="gifts.your-domain.example"
					autocomplete="off"
				/>
			</div>
			<Button type="submit">Add domain</Button>
		</form>

		{#if data.domains.length > 0}
			<ul class="mt-4 space-y-3">
				{#each data.domains as domain (domain.id)}
					<li class="rounded-card border border-line bg-surface-alt p-4">
						<div class="flex items-center justify-between gap-2">
							<span class="font-ui text-body font-medium text-ink">{domain.hostname}</span>
							{#if domain.verified}
								<span class="rounded-card bg-green-tint px-2 py-0.5 font-ui text-chip text-green"
									>Verified</span
								>
							{:else}
								<span class="rounded-card bg-amber-50 px-2 py-0.5 font-ui text-chip text-amber-700"
									>Not verified</span
								>
							{/if}
						</div>

						{#if !domain.verified}
							<!-- The unverified-domain DNS records are undesigned (the set draws only the
							     empty state); the existing structure is kept and dressed in the tokens
							     rather than redrawn (#234). -->
							<div class="mt-3 space-y-2 font-ui text-ui">
								<p class="text-ink-muted">Publish these DNS records, then verify:</p>
								<DomainDnsRecords
									hostname={domain.hostname}
									cnameTarget={domain.cname_target}
									verificationToken={domain.verification_token}
								/>
								{#if form?.verifiedId === domain.id && form?.nowVerified === false}
									<p class="text-amber-700">
										Not verified yet — DNS changes can take a while to propagate. Try again shortly.
									</p>
								{/if}
							</div>
						{/if}

						<div class="mt-3 flex gap-2">
							{#if !domain.verified}
								<!-- Verify is gated on a configured CNAME target: without one the domain
								     can't be served, so verifying it would be inconsistent (#239). -->
								<form method="post" action="?/verifyDomain" use:enhance>
									<input type="hidden" name="id" value={domain.id} />
									<Button type="submit" variant="secondary" disabled={!domain.cname_target}>
										Verify
									</Button>
								</form>
							{/if}
							<form method="post" action="?/removeDomain" use:enhance>
								<input type="hidden" name="id" value={domain.id} />
								<!-- Destructive, so it keeps a red signal; undesigned control kept restrained. -->
								<button
									type="submit"
									class="inline-flex h-12 items-center rounded-card border border-line bg-surface px-6 font-ui text-ui font-medium text-red-600 transition-colors hover:bg-red-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500"
								>
									Remove
								</button>
							</form>
						</div>
					</li>
				{/each}
			</ul>
		{:else}
			<p class="mt-4 rounded-card bg-primary-tint p-3 font-ui text-ui text-primary">
				No custom domains yet.
			</p>
		{/if}
	</section>
</div>
