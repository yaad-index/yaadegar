<script lang="ts">
	import { superForm } from 'sveltekit-superforms';
	import { Button } from 'bits-ui';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	const { form, enhance } = superForm(data.form);
</script>

<svelte:head>
	<title>Yaadegar</title>
	<meta name="description" content="API-first, self-hosted gift registry and wishlist." />
</svelte:head>

<main class="mx-auto max-w-xl p-8">
	<h1 class="text-2xl font-bold">Yaadegar</h1>
	<p class="mt-1 text-gray-600">Frontend foundations (Cut F1). No features yet.</p>

	<!-- Trivial usage of the form + component libraries; not a real feature. -->
	<form method="post" use:enhance class="mt-6 space-y-3">
		<input
			class="w-full rounded border p-2"
			name="name"
			bind:value={$form.name}
			placeholder="example field"
		/>
		<Button.Root class="rounded bg-black px-3 py-2 text-white">Example button</Button.Root>
	</form>
</main>
