<script lang="ts">
	// The list index / dashboard (#203), rebuilt on the foundations. Content only —
	// the create-list action and the server load keep their contracts; the shell
	// (top bar, 800px column, single main) is the (app) layout's (#205). The empty
	// state is rendered INLINE in place of the list region (the banner, heading and
	// create row stay), NOT through PageShell's isEmpty, because the empty copy
	// points at the create form above it. Item-preview thumbnails from the design
	// are omitted here: the list summary does not expose them (a backend change,
	// tracked separately).
	import { superForm } from 'sveltekit-superforms';
	import { resolve } from '$app/paths';
	import Button from '$lib/components/Button.svelte';
	import Card from '$lib/components/Card.svelte';
	import { accentFor } from '$lib/tokens';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	// superForm captures the initial form once and owns its reactivity thereafter.
	// svelte-ignore state_referenced_locally
	const { form, errors, enhance, submitting } = superForm(data.form);
</script>

<svelte:head><title>Your lists · Yaadegar</title></svelte:head>

{#snippet cardGift()}
	<svg
		width="20"
		height="20"
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="2"
		stroke-linecap="round"
		stroke-linejoin="round"
		aria-hidden="true"
	>
		<rect x="3" y="8" width="18" height="4" rx="1" />
		<path d="M12 8v13M5 12v7a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-7" />
		<path d="M12 8S9.5 3.5 7.5 4.5 7 8 12 8Zm0 0s2.5-4.5 4.5-3.5S17 8 12 8Z" />
	</svg>
{/snippet}

<!-- Welcome banner -->
<section class="flex items-start justify-between gap-4 rounded-card bg-surface-accent p-6">
	<div>
		<h1 class="font-display text-panel text-ink-heading">
			Welcome back to your safe space of wishes
		</h1>
		<p class="mt-2 max-w-lg font-ui text-body text-ink-muted">
			Create separate spaces for seasons, occasions, or dream items. Share them elegantly when
			you're ready.
		</p>
	</div>
	<!-- gold sparkle mark -->
	<svg
		class="shrink-0 text-gold"
		width="28"
		height="28"
		viewBox="0 0 24 24"
		fill="currentColor"
		aria-hidden="true"
	>
		<path d="M12 2l1.9 5.6L19.5 9l-5.6 1.9L12 16l-1.9-5.1L4.5 9l5.6-1.4L12 2z" />
	</svg>
</section>

<!-- Your lists -->
<div class="mt-8">
	<h2 class="font-display text-title text-ink-heading">Your lists</h2>
	<p class="mt-1 font-ui text-ui text-ink-muted">
		Organized collections of your desired gifts and curated memories
	</p>
</div>

<!-- Create-list row: a white bordered field (distinct from the auth screens' filled
	 inputs), a leading +, and the primary Create button beside it. -->
<form method="post" action="?/create" use:enhance class="mt-4 flex items-start gap-3">
	<div class="relative flex-1">
		<span class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-ink-muted">
			<svg
				width="18"
				height="18"
				viewBox="0 0 24 24"
				fill="none"
				stroke="currentColor"
				stroke-width="2"
				stroke-linecap="round"
				aria-hidden="true"
			>
				<path d="M12 5v14M5 12h14" />
			</svg>
		</span>
		<input
			class={`h-12 w-full rounded-card border bg-surface pl-10 pr-3 font-ui text-body text-ink placeholder:text-ink-muted focus:outline-none focus-visible:ring-2 ${
				$errors.title
					? 'border-red-500 focus-visible:ring-red-500'
					: 'border-line focus-visible:border-primary focus-visible:ring-primary'
			}`}
			name="title"
			placeholder="New list title…"
			aria-label="New list title"
			aria-invalid={$errors.title ? 'true' : undefined}
			aria-describedby={$errors.title ? 'create-error' : undefined}
			bind:value={$form.title}
		/>
	</div>
	<Button type="submit" disabled={$submitting}>Create</Button>
</form>
{#if $errors.title}
	<p id="create-error" class="mt-1 font-ui text-ui text-red-600" role="alert">{$errors.title}</p>
{/if}

<!-- The list region — or, in place of it, the empty state. -->
{#if data.lists.length > 0}
	<ul class="mt-6 space-y-4">
		{#each data.lists as list, i (list.id)}
			<li>
				<Card
					accent={accentFor(i)}
					href={resolve('/(app)/lists/[id]', { id: list.id ?? '' })}
					icon={cardGift}
				>
					<div class="flex items-center justify-between gap-3">
						<div class="min-w-0">
							<p class="truncate font-ui text-title font-medium text-ink-heading">{list.title}</p>
							<span
								class="mt-1 inline-block rounded-card bg-surface-alt px-2 py-0.5 font-ui text-chip text-ink-muted"
							>
								{list.item_count ?? 0} items
							</span>
						</div>
						<svg
							class="shrink-0 text-ink-muted"
							width="20"
							height="20"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="2"
							stroke-linecap="round"
							stroke-linejoin="round"
							aria-hidden="true"
						>
							<path d="M9 18l6-6-6-6" />
						</svg>
					</div>
				</Card>
			</li>
		{/each}
	</ul>
{:else}
	<div class="mt-6 flex flex-col items-center py-16 text-center">
		<div
			class="mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-primary-tint text-primary"
		>
			<svg
				width="32"
				height="32"
				viewBox="0 0 24 24"
				fill="none"
				stroke="currentColor"
				stroke-width="2"
				stroke-linecap="round"
				stroke-linejoin="round"
				aria-hidden="true"
			>
				<rect x="3" y="8" width="18" height="4" rx="1" />
				<path d="M12 8v13M5 12v7a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-7" />
				<path d="M12 8S9.5 3.5 7.5 4.5 7 8 12 8Zm0 0s2.5-4.5 4.5-3.5S17 8 12 8Z" />
			</svg>
		</div>
		<h3 class="font-display text-title text-ink-heading">No lists yet</h3>
		<p class="mt-2 max-w-sm font-ui text-body text-ink-muted">
			Create your first list above to start collecting gift ideas for someone special
		</p>
	</div>
{/if}

<!-- Footer note -->
<p class="mt-8 font-ui text-ui text-ink-muted">
	Your lists are private by default. Sharing configurations can be set per list.
</p>
