<script lang="ts">
	import { enhance } from '$app/forms';
	import { resolve } from '$app/paths';
	import { RESERVATION_PRIVACY_PROMISE } from '$lib/components/ReservationPrivacyNote.svelte';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();
</script>

<svelte:head><title>Reserved by me · Yaadegar</title></svelte:head>

<!-- The "Reserved by me" title is the 36px display rung (.display-title-md); the
     dashboard title is 40px and the list/settings titles 32px — same 700 weight,
     different measured size per role (#234, sizes pinned in the issue). -->
<h1 class="display-title-md font-display text-ink-heading">Reserved by me</h1>
<!-- The privacy promise is the product's core guarantee; it is single-sourced from
     ReservationPrivacyNote so the wording can't drift, and rendered inline here to
     match the export's one-sentence subline. -->
<p class="mt-2 font-ui text-body text-ink-muted">
	Gifts you've reserved across lists. {RESERVATION_PRIVACY_PROMISE}.
</p>

{#if form?.releaseError}
	<p class="mt-4 rounded-card bg-red-50 p-3 font-ui text-ui text-red-600" role="alert">
		{form.releaseError}
	</p>
{/if}

{#if data.reservations.length > 0}
	<ul class="mt-8 space-y-3">
		{#each data.reservations as r (r.reservation_id)}
			{@const notified = r.state === 'reserver_notified'}
			<li
				class="flex items-center justify-between gap-3 rounded-card border border-line bg-surface p-4"
			>
				<!-- The card carries NO thumbnail and NO price: the MyReservation payload has
				     neither, and the design draws both. Rather than invent a placeholder slot,
				     the card is laid out to read correctly without them; adding real images and
				     prices is a backend change, raised as a design-set question (#234). -->
				<div class="min-w-0">
					<a
						href={resolve('/l/[shareSlug]', { shareSlug: r.share_slug })}
						class="font-ui text-body font-medium text-ink-heading hover:underline">{r.item_name}</a
					>
					<div class="mt-1 flex flex-wrap items-center gap-2 font-ui text-ui text-ink-muted">
						<span>from {r.list_title}</span>
						{#if r.quantity > 1}<span>×{r.quantity}</span>{/if}
						<span aria-hidden="true">·</span>
						<!-- The export draws RESERVED on every row; the build has two states, so the
						     notified reservation keeps its own "Reminder sent" chip rather than being
						     flattened to match the single drawn state. Amber is the reserved-state tint
						     shared with the list view's availability chip. -->
						<span
							class={`rounded-card px-2 py-0.5 font-ui text-chip font-medium uppercase tracking-wide ${
								notified ? 'bg-amber-100 text-amber-800' : 'bg-amber-50 text-amber-700'
							}`}
						>
							{notified ? 'Reminder sent' : 'Reserved'}
						</span>
					</div>
				</div>
				<form method="post" action="?/release" use:enhance>
					<input type="hidden" name="reservation_id" value={r.reservation_id} />
					<!-- Not <Button variant="secondary">: the export gives Release rose (primary)
					     text on a bordered control, and the shared secondary variant hardcodes ink
					     text — overriding it by class is an unresolved Tailwind conflict here. Same
					     geometry as the secondary button, primary text. -->
					<button
						type="submit"
						class="inline-flex h-10 items-center gap-1.5 rounded-card border border-line bg-surface px-4 font-ui text-ui font-medium text-primary transition-colors hover:bg-surface-alt focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
					>
						<svg
							width="16"
							height="16"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="2"
							stroke-linecap="round"
							stroke-linejoin="round"
							aria-hidden="true"
						>
							<circle cx="12" cy="12" r="10" />
							<line x1="15" y1="9" x2="9" y2="15" />
							<line x1="9" y1="9" x2="15" y2="15" />
						</svg>
						Release
					</button>
				</form>
			</li>
		{/each}
	</ul>
{:else}
	<!-- Empty state is a designed state, not a fallback. It diverges from the shared
	     EmptyState component (a rose-tint circle, not beige; a tip pill, not a button),
	     so it is built in place to match the export rather than bent out of the shared
	     one — flagged as a design-system question (#234). -->
	<div class="mt-8 rounded-card border border-line bg-surface px-6 py-16">
		<div class="mx-auto max-w-sm text-center">
			<div
				class="mx-auto mb-5 flex h-16 w-16 items-center justify-center rounded-full bg-primary-tint text-primary"
			>
				<svg
					width="28"
					height="28"
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
			<!-- text-title (20px). The export's empty-state heading measures ~22px, just
			     above this rung; it is kept on the existing rung (also the shared
			     EmptyState's heading size) rather than minting a token, and the ~2px is
			     flagged for the designer (#234). -->
			<h2 class="font-display text-title text-ink-heading">You haven't reserved anything yet</h2>
			<p class="mt-2 font-ui text-body text-ink-muted">
				When you reserve a gift from someone's list, it'll show up here.
			</p>
			<div class="mt-6 flex justify-center">
				<span class="rounded-full bg-gold-tint px-4 py-2 font-ui text-ui font-medium text-ink">
					💡 Check your friends' share links to reserve your first gift!
				</span>
			</div>
		</div>
	</div>
{/if}
