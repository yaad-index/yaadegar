<script lang="ts">
	import { enhance } from '$app/forms';
	import { resolve } from '$app/paths';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();
</script>

<svelte:head><title>Users · Yaadegar admin</title></svelte:head>

<a href={resolve('/admin')} class="text-sm text-gray-500 hover:underline">← Tenants</a>
<h1 class="mt-1 text-2xl font-bold">Users</h1>

{#if form?.actionError}
	<p class="mt-3 rounded bg-red-50 p-2 text-sm text-red-700" role="alert">{form.actionError}</p>
{/if}
{#if form?.created}
	<p class="mt-3 rounded bg-green-50 p-2 text-sm text-green-700" role="status">
		Created {form.created}.
	</p>
{/if}

<section class="mt-4 rounded border p-3">
	<h2 class="text-sm font-medium">Add a user</h2>
	<form method="post" action="?/create" use:enhance class="mt-2 flex flex-wrap items-center gap-2">
		<input
			class="rounded border p-2 text-sm"
			name="email"
			placeholder="email@example.com"
			autocomplete="off"
		/>
		<select name="role" class="rounded border p-2 text-sm">
			<option value="owner">Owner</option>
			<option value="giver">Giver</option>
		</select>
		<button class="rounded bg-black px-3 py-2 text-sm text-white">Add</button>
	</form>
	<p class="mt-1 text-xs text-gray-500">
		The user is created with no password; they set credentials later via a login method.
	</p>
</section>

<ul class="mt-4 space-y-3">
	{#each data.users as user (user.id)}
		<li class="rounded border p-3">
			<div class="flex items-center justify-between">
				<div>
					<!-- Name is shown when present; email is the stable identifier and always
					     shown (secondary when a name exists, primary when it does not). -->
					{#if user.name}
						<span class="font-medium">{user.name}</span>
						<span class="ml-2 text-xs text-gray-500">{user.email}</span>
					{:else}
						<span class="font-medium">{user.email}</span>
					{/if}
					{#if user.is_admin}
						<span class="ml-2 rounded bg-indigo-100 px-2 py-0.5 text-xs text-indigo-800">Admin</span
						>
					{/if}
					{#if user.banned}
						<span class="ml-2 rounded bg-red-100 px-2 py-0.5 text-xs text-red-800">Banned</span>
					{/if}
				</div>
				<span class="text-xs text-gray-500">{user.role}</span>
			</div>
			<div class="mt-2 flex flex-wrap items-center gap-2">
				<!-- Change role. Demotion to giver returns a 409 when the account owns
				     lists; the message is surfaced above. -->
				<form method="post" action="?/update" use:enhance class="flex items-center gap-1">
					<input type="hidden" name="user_id" value={user.id} />
					<select
						name="role"
						class="rounded border p-1 text-xs"
						onchange={(e) => e.currentTarget.form?.requestSubmit()}
					>
						<option value="owner" selected={user.role === 'owner'}>Owner</option>
						<option value="giver" selected={user.role === 'giver'}>Giver</option>
					</select>
				</form>
				<form method="post" action="?/update" use:enhance>
					<input type="hidden" name="user_id" value={user.id} />
					<input type="hidden" name="banned" value={user.banned ? 'false' : 'true'} />
					<button class="rounded border px-2 py-1 text-xs">
						{user.banned ? 'Unban' : 'Ban'}
					</button>
				</form>
			</div>
		</li>
	{:else}
		<li class="text-sm text-gray-500">No users in this tenant.</li>
	{/each}
</ul>
