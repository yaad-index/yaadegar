<script lang="ts">
	import { enhance } from '$app/forms';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import DomainDnsRecords from '$lib/components/DomainDnsRecords.svelte';
	import Field from '$lib/components/Field.svelte';
	import Button from '$lib/components/Button.svelte';
	import { formatDay, tokenState } from '$lib/accessTokens';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();
	// After a toggle save the action returns fresh settings; otherwise use the load.
	const settings = $derived(form?.settings ?? data.settings);

	// The shared page (#308). A freshly created or rotated key comes back on the
	// action; otherwise it is whatever the load found — null meaning the owner has
	// not created one, which is the ordinary state rather than a missing value.
	const ownerKey = $derived(form?.ownerKey ?? data.ownerKey);
	const ownerUrl = $derived(ownerKey ? `${page.url.origin}/o/${ownerKey}` : '');
	let ownerCopied = $state<'idle' | 'ok' | 'fail'>('idle');
	let ownerCopyTimer: ReturnType<typeof setTimeout> | undefined;
	async function copyOwnerUrl() {
		clearTimeout(ownerCopyTimer);
		try {
			await navigator.clipboard.writeText(ownerUrl);
			ownerCopied = 'ok';
		} catch {
			ownerCopied = 'fail';
		}
		ownerCopyTimer = setTimeout(() => (ownerCopied = 'idle'), 2500);
	}

	// A newly created personal access token's value (ADR-0016). It exists only in
	// this action result: it is shown once and the app keeps no copy.
	const createdToken = $derived(form?.createdToken ?? '');
	let tokenField = $state<HTMLInputElement>();
	let tokenCopied = $state<'idle' | 'ok' | 'manual'>('idle');
	let tokenCopyTimer: ReturnType<typeof setTimeout> | undefined;
	// The clipboard API is missing on plain http, and the value cannot be shown
	// again, so a failed copy must leave a way forward: the text is selected and the
	// page says to copy it by hand. That state stays until the next copy attempt.
	async function copyToken() {
		clearTimeout(tokenCopyTimer);
		try {
			await navigator.clipboard.writeText(createdToken);
			tokenCopied = 'ok';
			tokenCopyTimer = setTimeout(() => (tokenCopied = 'idle'), 2500);
		} catch {
			tokenCopied = 'manual';
			tokenField?.focus();
			tokenField?.select();
		}
	}

	// Revoking cannot be undone, so it asks first.
	function confirmRevoke(e: SubmitEvent, name: string) {
		if (!confirm(`Revoke “${name}”? Anything using it stops working, and this can't be undone.`)) {
			e.preventDefault();
		}
	}
	const activeTokenCount = $derived(data.tokens.filter((t) => t.active).length);
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

	<!-- The shared page (#308): one durable link for every list the owner chooses to
	     show, instead of one link per list. Sits next to Display name because that is
	     the name this page carries. -->
	<section class="rounded-card border border-line bg-surface p-6">
		<h2 class="font-display text-title text-ink-heading">Your shared page</h2>
		<p class="mt-1 font-ui text-ui text-ink-muted">
			One link that shows every list you've chosen to share, so you can send it once instead of
			sending a new link each time you make a list.
		</p>

		{#if form?.ownerKeyCreated}
			<p class="mt-4 rounded-card bg-green-tint p-3 font-ui text-ui text-green" role="status">
				Your shared page is ready.
			</p>
		{/if}
		{#if form?.ownerKeyRotated}
			<p class="mt-4 rounded-card bg-green-tint p-3 font-ui text-ui text-green" role="status">
				New link created. The previous one no longer opens.
			</p>
		{/if}
		{#if form?.ownerKeyError}
			<p class="mt-4 rounded-card bg-red-50 p-3 font-ui text-ui text-red-600" role="alert">
				{form.ownerKeyError}
			</p>
		{/if}

		{#if ownerKey}
			<div class="mt-4 flex flex-wrap items-center gap-3">
				<input
					class="min-w-0 flex-1 rounded-card border border-line bg-surface-alt px-3 py-2 font-ui text-ui text-ink"
					value={ownerUrl}
					readonly
					aria-label="Your shared page link"
				/>
				<Button type="button" variant="secondary" onclick={copyOwnerUrl}>
					{ownerCopied === 'ok' ? 'Copied' : ownerCopied === 'fail' ? 'Copy failed' : 'Copy'}
				</Button>
				<!-- A runtime absolute URL on the tenant origin, built from the key the
				     backend minted; resolve() has no route to express it. Same reason the
				     list page's share link is constructed rather than resolved. -->
				<!-- eslint-disable svelte/no-navigation-without-resolve -->
				<a
					href={ownerUrl}
					target="_blank"
					rel="noopener"
					class="font-ui text-ui text-primary transition-colors hover:text-primary-hover">Open</a
				>
				<!-- eslint-enable svelte/no-navigation-without-resolve -->
			</div>

			<p class="mt-3 font-ui text-ui text-ink-muted">
				A list appears here only when you turn on <span class="text-ink"
					>Show on my shared page</span
				> in that list's settings. Everything else stays off it, and every list's own share link keeps
				working either way.
			</p>
			<!-- Stated as the rule rather than as this account's current state: the
			     display name defaults to the account email, and the frontend is not told
			     the email, so it cannot tell the two apart. The backend can and does —
			     it withholds the name in exactly that case. -->
			<p class="mt-2 font-ui text-ui text-ink-muted">
				Your page is headed by your display name once you've set one above. Until then it stays
				unnamed — your email address is never shown on it.
			</p>

			<form method="post" action="?/ownerKey" use:enhance class="mt-4">
				<input type="hidden" name="rotating" value="true" />
				<Button type="submit" variant="secondary">Create a new link</Button>
				<p class="mt-2 font-ui text-ui text-ink-muted">
					Use this if you've shared the link too widely. It replaces the link above, and anyone
					holding the old one will no longer be able to open your page. Your lists themselves are
					not affected.
				</p>
			</form>
		{:else}
			<form method="post" action="?/ownerKey" use:enhance class="mt-4">
				<Button type="submit">Create my shared page</Button>
				<p class="mt-2 font-ui text-ui text-ink-muted">
					Nothing is shared until you create this and then choose which lists to show. You can
					replace the link later at any time.
				</p>
			</form>
		{/if}
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
				{#if form.activeTokens}
					{form.activeTokens === 1
						? 'Your 1 access token still works.'
						: `Your ${form.activeTokens} access tokens still work.`}
					<a href="#access-tokens" class="underline">Review them</a>
				{/if}
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

	<!-- Personal access tokens (ADR-0016 §7). Undesigned: built from the same card,
	     field, button and status pieces as the sections around it. -->
	<section id="access-tokens" class="rounded-card border border-line bg-surface p-6">
		<h2 class="font-display text-title text-ink-heading">Access tokens</h2>
		<p class="mt-1 font-ui text-ui text-ink-muted">
			A token lets a script or another program use your account without your password. It can do
			anything you can do with your lists and items, so keep it as safe as a password. A token
			cannot create or revoke tokens, or change your password.
		</p>

		{#if data.tokensRevoked}
			<p class="mt-4 rounded-card bg-amber-50 p-3 font-ui text-ui text-amber-700" role="status">
				Your password was reset, so {data.tokensRevoked === 1
					? 'your access token was'
					: `all ${data.tokensRevoked} of your access tokens were`} revoked. Create new ones for anything
				that still needs access.
			</p>
		{/if}
		{#if form?.tokenRevoked}
			<p class="mt-4 rounded-card bg-green-tint p-3 font-ui text-ui text-green" role="status">
				Token revoked. It stops working immediately.
			</p>
		{/if}
		{#if form?.tokenError}
			<div class="mt-4 rounded-card bg-red-50 p-3 font-ui text-ui text-red-600" role="alert">
				<p>{form.tokenError}</p>
				{#if 'tokenNeedsSignIn' in form && form.tokenNeedsSignIn}
					<form method="post" action="/logout?return_to=/settings" class="mt-2">
						<Button type="submit" variant="secondary">Sign in again</Button>
					</form>
				{/if}
			</div>
		{/if}

		{#if createdToken}
			<div class="mt-4 rounded-card border border-line bg-green-tint p-4" role="status">
				<p class="font-ui text-ui font-medium text-green">
					Token “{form?.createdTokenName}” created. Copy it now: you won't be able to see it again.
				</p>
				<div class="mt-3 flex flex-wrap items-center gap-3">
					<input
						bind:this={tokenField}
						class="min-w-0 flex-1 rounded-card border border-line bg-surface px-3 py-2 font-mono text-ui text-ink"
						value={createdToken}
						readonly
						aria-label="Your new access token"
					/>
					<Button type="button" variant="secondary" onclick={copyToken}>
						{tokenCopied === 'ok' ? 'Copied' : 'Copy'}
					</Button>
				</div>
				{#if tokenCopied === 'manual'}
					<p class="mt-2 font-ui text-ui text-ink" role="alert">
						This browser couldn't copy it for you. The token is selected above: copy it with Ctrl+C
						(⌘C on a Mac) before you leave this page.
					</p>
				{/if}
			</div>
		{/if}

		<form method="post" action="?/createToken" use:enhance class="mt-4 space-y-4">
			<Field label="Name" name="name" maxlength={100} placeholder="Backup script" required />
			<label class="block font-ui text-ui text-ink">
				<span class="font-medium">Expires</span>
				<select
					name="expiry"
					required
					class="mt-1 block w-full rounded-card border border-line bg-surface px-3 py-2 font-ui text-body text-ink"
				>
					<option value="" selected disabled>Choose…</option>
					{#each data.tokenExpiryDays as days (days)}
						<option value={String(days)}>In {days} days</option>
					{/each}
					<option value="never">Never</option>
				</select>
			</label>
			<p class="font-ui text-ui text-ink-muted">
				For your security, creating a token needs a sign-in within the last 10 minutes.
			</p>
			<Button type="submit">Create token</Button>
		</form>

		{#if data.tokens.length > 0}
			<p class="mt-6 font-ui text-ui text-ink-muted">
				{activeTokenCount} of 20 active tokens.
			</p>
			<ul class="mt-2 space-y-3">
				{#each data.tokens as t (t.id)}
					{@const state = tokenState(t)}
					<li class="rounded-card border border-line bg-surface-alt p-4">
						<div class="flex items-center justify-between gap-2">
							<span class="font-ui text-body font-medium text-ink">
								{t.name} <span class="font-mono text-ink-muted">…{t.last4}</span>
							</span>
							{#if state === 'active'}
								<span class="rounded-card bg-green-tint px-2 py-0.5 font-ui text-chip text-green"
									>Active</span
								>
							{:else if state === 'revoked'}
								<span class="rounded-card bg-red-50 px-2 py-0.5 font-ui text-chip text-red-600"
									>Revoked</span
								>
							{:else}
								<span class="rounded-card bg-amber-50 px-2 py-0.5 font-ui text-chip text-amber-700"
									>Expired</span
								>
							{/if}
						</div>
						<p class="mt-2 font-ui text-ui text-ink-muted">
							Created {formatDay(t.created_at)} ·
							{t.expires_at ? `Expires ${formatDay(t.expires_at)}` : 'Never expires'} ·
							{t.last_used_at ? `Last used ${formatDay(t.last_used_at)}` : 'Never used'}
						</p>
						{#if state === 'active'}
							<form
								method="post"
								action="?/revokeToken"
								use:enhance
								class="mt-3"
								onsubmit={(e) => confirmRevoke(e, t.name)}
							>
								<input type="hidden" name="id" value={t.id} />
								<button
									type="submit"
									class="inline-flex h-12 items-center rounded-card border border-line bg-surface px-6 font-ui text-ui font-medium text-red-600 transition-colors hover:bg-red-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500"
								>
									Revoke
								</button>
							</form>
						{/if}
					</li>
				{/each}
			</ul>
		{:else}
			<p class="mt-4 rounded-card bg-primary-tint p-3 font-ui text-ui text-primary">
				No access tokens yet.
			</p>
		{/if}
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
