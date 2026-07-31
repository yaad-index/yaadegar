<script lang="ts">
	import { resolve } from '$app/paths';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
</script>

<svelte:head><title>Tenants · Yaadegar admin</title></svelte:head>

<h1 class="text-2xl font-bold">Tenants</h1>
<p class="mt-1 text-sm text-gray-600">Pick a tenant to manage its users.</p>

<ul class="mt-4 divide-y rounded border">
	{#each data.tenants as tenant (tenant.id)}
		<li>
			<a
				href={resolve('/admin/tenants/[tenantId]', { tenantId: tenant.id ?? '' })}
				class="flex items-center justify-between p-3 hover:bg-gray-50"
			>
				<span class="font-medium">{tenant.subdomain}</span>
				<span class="text-xs text-gray-400">Manage users →</span>
			</a>
		</li>
	{:else}
		<li class="p-3 text-sm text-gray-500">No tenants yet.</li>
	{/each}
</ul>
