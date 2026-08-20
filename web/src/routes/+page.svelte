<script lang="ts">
	// The public marketing landing (#236), the site root for a signed-out visitor. This
	// first pass is the header + hero + trust strip; the remaining sections (how it works,
	// built differently, self-hosting, closing CTA, footer) land in later passes. Copy is
	// what the project actually does TODAY (maintainer decision): self-hosted, Docker
	// Compose, SQLite for dev / Postgres for production — no one-click-deploy promise the
	// repo has no artifact for.
	import { resolve } from '$app/paths';

	// The canonical source repo (the Go module path — already public throughout the tree).
	const REPO_URL = 'https://github.com/yaad-index/yaadegar';

	// The hero's product mockup is a hand-built REPLICA, not a screenshot: a screenshot of
	// the running app goes stale silently, the exact drift-you-can't-see failure the
	// redesign is guarding against; a replica breaks visibly when the UI changes. Its
	// content is deliberately INVENTED (no real handle, name, or address on a public page).
	const mockItems = [
		{
			name: 'Ceramic Coffee Dripper',
			note: 'Earthy brown clay finish. Perfect for morning brews.',
			state: 'reserved' as const
		},
		{
			name: 'Waffle-Knit Rust Throw Blanket',
			note: 'Something warm for those autumn evenings on the couch.',
			state: 'claim' as const
		}
	];
</script>

<svelte:head>
	<title>Yaadegar — the open-source surprise wishlist</title>
	<meta
		name="description"
		content="Yaadegar is a friendly, self-hosted gift list web app. Share your wishlist without ruining the surprise — givers coordinate and reserve secretly while you stay in the dark."
	/>
</svelte:head>

<!-- Marketing header: its own nav (not the signed-in app shell). The section links are
     same-page anchors whose targets land in later passes. -->
<header class="border-b border-divider bg-page">
	<div class="mx-auto flex h-20 max-w-7xl items-center justify-between px-6">
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

		<!-- eslint-disable svelte/no-navigation-without-resolve -- same-page anchors and the
		     external source-repo link; neither is a SvelteKit route resolve() can express. -->
		<nav class="hidden items-center gap-8 font-ui text-ui text-ink-muted sm:flex">
			<a class="transition-colors hover:text-ink" href="#how-it-works">How it works</a>
			<a class="transition-colors hover:text-ink" href="#features">Features</a>
			<a class="transition-colors hover:text-ink" href="#self-hosting">Self-hosting</a>
			<a
				class="flex items-center gap-1.5 transition-colors hover:text-ink"
				href={REPO_URL}
				rel="noreferrer"
				target="_blank"
			>
				<svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
					<path
						d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0016 8c0-4.42-3.58-8-8-8z"
					/>
				</svg>
				GitHub
			</a>
		</nav>
		<!-- eslint-enable svelte/no-navigation-without-resolve -->

		<!-- Get started stays visible on mobile (the inline nav links collapse below sm);
		     the wordmark + this button are the mobile header. -->
		<a
			class="inline-flex h-11 items-center justify-center rounded-full bg-primary px-5 font-ui text-ui font-medium text-white transition-colors hover:bg-primary-hover sm:px-6"
			href={resolve('/login')}>Get started</a
		>
	</div>
</header>

<!-- Hero: two columns on desktop (copy left, product mockup right), stacked on mobile. -->
<section class="bg-page">
	<div class="mx-auto grid max-w-7xl gap-12 px-6 py-16 lg:grid-cols-2 lg:items-center lg:py-24">
		<div>
			<p
				class="flex items-center gap-2 font-ui text-ui font-medium uppercase tracking-wide text-primary"
			>
				<svg
					width="16"
					height="16"
					viewBox="0 0 24 24"
					fill="currentColor"
					aria-hidden="true"
					class="text-gold"
				>
					<path
						d="M12 2l2.9 6.26L21.5 9l-5 4.87L17.8 21 12 17.3 6.2 21l1.3-7.13-5-4.87 6.6-.74L12 2z"
					/>
				</svg>
				The open source surprise wishlist
			</p>

			<h1 class="landing-hero-title mt-5 font-display text-ink-heading">
				Wishes worth keeping<br />— surprises kept safe
			</h1>

			<p class="mt-6 max-w-xl font-ui text-[18px] leading-[1.6] text-ink-muted">
				Yaadegar is a friendly, self-hosted gift list web app. Share your wishlist with friends and
				family without ruining the magic — givers can coordinate and reserve secretly, while you
				stay blissfully in the dark.
			</p>

			<div class="mt-8 flex flex-wrap items-center gap-3">
				<a
					class="inline-flex h-12 items-center justify-center rounded-full bg-primary px-7 font-ui text-body font-medium text-white transition-colors hover:bg-primary-hover"
					href={resolve('/login')}>Create your list</a
				>
				<!-- eslint-disable svelte/no-navigation-without-resolve -- external source-repo link. -->
				<a
					class="inline-flex h-12 items-center justify-center gap-2 rounded-full border border-line bg-surface px-6 font-ui text-body font-medium text-ink transition-colors hover:bg-surface-alt"
					href={REPO_URL}
					rel="noreferrer"
					target="_blank"
				>
					<svg width="18" height="18" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
						<path
							d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0016 8c0-4.42-3.58-8-8-8z"
						/>
					</svg>
					View on GitHub
				</a>
				<!-- eslint-enable svelte/no-navigation-without-resolve -->
			</div>
		</div>

		<!-- Product mockup replica (invented content) inside a browser chrome. -->
		<div class="rounded-2xl border border-line bg-surface shadow-xl">
			<div class="flex items-center gap-2 border-b border-divider px-4 py-3">
				<span class="flex gap-1.5" aria-hidden="true">
					<span class="h-3 w-3 rounded-full bg-primary"></span>
					<span class="h-3 w-3 rounded-full bg-gold"></span>
					<span class="h-3 w-3 rounded-full bg-green"></span>
				</span>
				<span
					class="mx-auto flex items-center gap-1.5 rounded-md bg-surface-alt px-3 py-1 font-ui text-chip text-ink-muted"
				>
					<svg
						width="11"
						height="11"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
						aria-hidden="true"
					>
						<rect x="3" y="11" width="18" height="11" rx="2" /><path d="M7 11V7a5 5 0 0 1 10 0v4" />
					</svg>
					yaadegar.gift/sarah-home
				</span>
			</div>
			<div class="p-5">
				<p class="font-display text-display-sm font-semibold text-ink-heading">
					Sarah's Housewarming List
				</p>
				<p class="mt-1 font-ui text-ui text-ink-muted">
					Help me fill our cozy new flat with warmth!
				</p>
				<ul class="mt-4 space-y-3">
					{#each mockItems as item (item.name)}
						<li class="flex items-start gap-3 rounded-card border border-line bg-surface p-3">
							<span
								class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg bg-primary-tint text-primary"
								aria-hidden="true"
							>
								<svg
									width="20"
									height="20"
									viewBox="0 0 24 24"
									fill="none"
									stroke="currentColor"
									stroke-width="2"
									stroke-linecap="round"
									stroke-linejoin="round"
								>
									<rect x="3" y="8" width="18" height="4" rx="1" /><path
										d="M12 8v13M5 12v7a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-7"
									/><path d="M12 8S9.5 3.5 7.5 4.5 7 8 12 8Zm0 0s2.5-4.5 4.5-3.5S17 8 12 8Z" />
								</svg>
							</span>
							<div class="min-w-0 flex-1">
								<div class="flex items-start justify-between gap-2">
									<span class="font-ui text-ui font-medium text-ink-heading">{item.name}</span>
									{#if item.state === 'reserved'}
										<span
											class="shrink-0 rounded-card bg-amber-50 px-2 py-0.5 font-ui text-chip text-amber-700"
										>
											● Reserved
										</span>
									{:else}
										<span
											class="shrink-0 rounded-card bg-gold-tint px-2 py-0.5 font-ui text-chip text-gold"
										>
											Claim Gift
										</span>
									{/if}
								</div>
								<p class="mt-0.5 font-ui text-chip text-ink-muted">{item.note}</p>
							</div>
						</li>
					{/each}
				</ul>
			</div>
		</div>
	</div>

	<!-- Trust strip: three quick assurances. The deploy line is true-today — Docker
	     Compose with SQLite (dev) or Postgres (prod), NOT a one-click-deploy promise. -->
	<div class="border-y border-divider bg-surface-accent">
		<div
			class="mx-auto flex max-w-7xl flex-col gap-3 px-6 py-4 font-ui text-ui text-ink-muted sm:flex-row sm:items-center sm:justify-between"
		>
			<span class="flex items-center gap-2">
				<svg
					width="15"
					height="15"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="2"
					aria-hidden="true"
				>
					<rect x="3" y="11" width="18" height="11" rx="2" /><path d="M7 11V7a5 5 0 0 1 10 0v4" />
				</svg>
				No tracking. No third-party data sales.
			</span>
			<span class="font-medium text-ink">Free &amp; Open Source</span>
			<span class="flex items-center gap-2">
				<svg
					width="15"
					height="15"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="2"
					stroke-linecap="round"
					stroke-linejoin="round"
					aria-hidden="true"
				>
					<path
						d="M4.5 16.5c-1.5 1.26-2 5-2 5s3.74-.5 5-2c.71-.84.7-2.13-.09-2.91a2.18 2.18 0 0 0-2.91-.09z"
					/><path
						d="M12 15l-3-3a22 22 0 0 1 2-3.95A12.88 12.88 0 0 1 22 2c0 2.72-.78 7.5-6 11a22.35 22.35 0 0 1-4 2z"
					/>
				</svg>
				Docker Compose · SQLite or Postgres
			</span>
		</div>
	</div>
</section>

<style>
	/* Landing-LOCAL display type (#236 D2): the hero sits above the app's 40px top rung
	   and is used nowhere else, so it lives here rather than growing the shared scale with
	   a rung no app screen uses. Sized to the 1440px design export; pinned by matching the
	   rendered word-advance, not eyeballed. */
	.landing-hero-title {
		font-size: 54px;
		font-weight: 700;
		line-height: 1.16;
	}
	@media (max-width: 640px) {
		.landing-hero-title {
			font-size: 38px;
		}
	}
</style>
