<script lang="ts">
	// The owner page (#308): a giver-facing index of one owner's published lists.
	// Read-only — there is no reserve or chip-in surface here; each row links through
	// to that list's own public view, which already carries the giver flows.
	//
	// It borrows the giver surface's shell (wordmark-only header, centred column) from
	// the per-list public view rather than the (app) layout, because a visitor here is
	// not signed in. Rows reuse the dashboard's card idiom — Card + ItemPreviewCluster
	// + accentFor — so a list looks like itself on both sides of the share.
	import { resolve } from '$app/paths';
	import Card from '$lib/components/Card.svelte';
	import GiftGlyph from '$lib/components/GiftGlyph.svelte';
	import ItemPreviewCluster from '$lib/components/ItemPreviewCluster.svelte';
	import { accentFor } from '$lib/tokens';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// The heading names the owner only when they chose a name. Without one the page
	// stays unattributed rather than falling back to the account email.
	const heading = $derived(data.displayName ? `${data.displayName}'s lists` : 'Shared lists');
</script>

<svelte:head>
	<title>{heading} · Yaadegar</title>
	<meta property="og:title" content={heading} />
	<meta property="og:description" content="Gift lists shared on Yaadegar." />
</svelte:head>

{#snippet cardGift()}
	<GiftGlyph />
{/snippet}

<!-- Wordmark-only header, matching the per-list giver surface: no signed-in nav. -->
<header class="border-b border-divider bg-page">
	<div class="flex items-center px-6 py-4 sm:px-10">
		<a
			href={resolve('/')}
			class="flex items-center gap-2.5 font-display text-display-sm font-semibold text-primary"
		>
			<span
				class="flex h-8 w-8 items-center justify-center rounded-lg bg-primary text-xl leading-none text-white"
				aria-hidden="true">Y</span
			>
			Yaadegar
		</a>
	</div>
</header>

<main class="mx-auto max-w-2xl px-4 py-10">
	<h1 class="display-title-md font-display text-ink-heading">{heading}</h1>

	{#if data.lists.length > 0}
		<p class="mt-3 font-ui text-body text-ink-muted">
			Everything shared in one place. Open a list to reserve a gift.
		</p>
		<ul class="mt-6 space-y-4">
			{#each data.lists as list, i (list.share_slug)}
				<li>
					<Card
						accent={accentFor(i)}
						href={resolve('/l/[shareSlug]', { shareSlug: list.share_slug ?? '' })}
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
							<div class="flex shrink-0 items-center gap-3">
								<ItemPreviewCluster
									previews={list.item_previews ?? []}
									total={list.item_count ?? 0}
									accent={accentFor(i)}
								/>
								<svg
									class="text-ink-muted"
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
						</div>
					</Card>
				</li>
			{/each}
		</ul>

		<!-- The same closing promise the per-list giver surface makes, so it does not
		     first appear only after a visitor clicks through. -->
		<p class="mt-8 rounded-card bg-surface-accent p-4 text-center font-ui text-ui text-ink-muted">
			Reserving or chipping in keeps the surprise — the owner never sees who did either.
		</p>
	{:else}
		<!-- A real page with nothing published yet. Said plainly, because the visitor
		     here may well be the owner checking their own link, and "nothing published"
		     has to be distinguishable from "wrong link" (which is a 404). -->
		<div class="mt-6 flex flex-col items-center py-16 text-center">
			<div
				class="mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-primary-tint text-primary"
			>
				<GiftGlyph size={32} />
			</div>
			<h2 class="font-display text-title text-ink-heading">Nothing shared yet</h2>
			<p class="mt-2 max-w-sm font-ui text-body text-ink-muted">
				This page works, but no lists have been added to it yet. Lists appear here once their owner
				chooses to show them.
			</p>
		</div>
	{/if}
</main>
