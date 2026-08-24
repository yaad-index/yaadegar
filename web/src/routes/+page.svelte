<script lang="ts">
	// The public marketing landing (#236), the site root for a signed-out visitor. All
	// eight sections are now here: header, hero, trust strip, how-it-works, built-
	// differently, pull quote, a second product mockup, self-hosting, the closing CTA, and
	// the footer. Copy is what the project actually does TODAY (maintainer decision):
	// self-hosted, Docker Compose, SQLite for dev / Postgres for production — no one-click
	// or Fly.io/Railway promise the repo has no artifact for, and no theming claim the app
	// doesn't yet support.
	import { resolve } from '$app/paths';
	import GiftGlyph from '$lib/components/GiftGlyph.svelte';
	import type { PageData } from './$types';

	// `custom` is populated only when the operator set YAADEGAR_ROOT_PAGE=custom (#256):
	// the hero headline, sub-head, CTA, and the trust line then come from operator strings,
	// already escaped/allowlisted server-side (ADR-0015). In bundled and login modes it is
	// absent, so the page renders its shipped copy byte-for-byte.
	let { data }: { data: PageData } = $props();
	const custom = $derived(data.custom ?? null);

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
			state: 'available' as const
		}
	];

	// The four "how it works" steps; step 3 is emphasised (filled number badge + border).
	const steps = [
		{
			n: 1,
			title: 'Make your wishlist',
			body: 'Write down whatever makes your heart sing. Add names, links, and optional photos.'
		},
		{
			n: 2,
			title: 'Share the link',
			body: 'Send your private list URL to friends, family, or publish it. No authentication wall for them.'
		},
		{
			n: 3,
			title: 'Friends reserve secretly',
			body: "Givers reserve items so duplicates don't happen. The system coordinates without spoiling who did it.",
			featured: true
		},
		{
			n: 4,
			title: 'Surprise stays safe',
			body: 'When you look at your own wishlist, everything looks active. No spoilers, ever.'
		}
	];

	// The "take a look inside" section is a SECOND hand-built REPLICA (same rule as the
	// hero: invented content, no real handle, name, or address on a public page). It's a
	// giver's view of a shared list, so it speaks the app's real vocabulary — "Reserved" /
	// "Available" and the "Wants N" quantity — never a word the product doesn't use
	// (the app has no "Claimed" or "Desired"). Item images are the app's gift-glyph
	// placeholder on a category-accent preview tint (the #207 tints), not stock photos:
	// a public page shouldn't carry licensed photography, and this leaves the
	// glyph-vs-photo question open rather than settling it with real images.
	const showcaseItems = [
		{
			name: 'The Winter Poems Anthology',
			note: 'Hardcover collection of deep, nature-soaked verse.',
			price: '$32',
			reserved: true,
			wants: 1,
			tint: 'bg-primary-preview',
			icon: 'text-primary'
		},
		{
			name: 'Wooden Balance Desk Lamp',
			note: 'Warm, glowing ambient light for late-night journaling.',
			price: '$89',
			reserved: false,
			wants: 1,
			tint: 'bg-gold-preview',
			icon: 'text-gold'
		},
		{
			name: 'Linen Bound Sketchbook',
			note: 'Lay-flat blank heavy paper. Beautiful texture.',
			price: '$18',
			reserved: false,
			wants: 2,
			tint: 'bg-green-preview',
			icon: 'text-green'
		}
	];

	// Footer link columns. Every href points at something that exists TODAY, checked
	// against the repo: the container image lives on GHCR (the page must not say "Docker
	// Hub", where nothing is published); GitHub Discussions is not enabled, so the
	// community column links the issue tracker and the self-hosting guide rather than a
	// dead Discussions tab; and the licence is the MIT LICENSE file GitHub now detects.
	const footerColumns = [
		{
			heading: 'Project',
			links: [
				{ label: 'Source Code', href: REPO_URL },
				{ label: 'Container Image', href: `${REPO_URL}/pkgs/container/yaadegar` },
				{ label: 'Releases', href: `${REPO_URL}/releases` }
			]
		},
		{
			heading: 'Community',
			links: [
				{ label: 'Issue Tracker', href: `${REPO_URL}/issues` },
				{ label: 'Self-hosting Guide', href: `${REPO_URL}/blob/main/docs/self-hosting.md` },
				{ label: 'MIT License', href: `${REPO_URL}/blob/main/LICENSE` }
			]
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
				{#if custom}{custom.headline}{:else}Wishes worth keeping<br />— surprises kept safe{/if}
			</h1>

			<p class="mt-6 max-w-xl font-ui text-[18px] leading-[1.6] text-ink-muted">
				{#if custom}{custom.subhead}{:else}Yaadegar is a friendly, self-hosted gift list web app.
					Share your wishlist with friends and family without ruining the magic — givers can
					coordinate and reserve secretly, while you stay blissfully in the dark.{/if}
			</p>

			<div class="mt-8 flex flex-wrap items-center gap-3">
				<!-- eslint-disable svelte/no-navigation-without-resolve -- the CTA href is operator-configured
				     in custom mode (allowlisted to http/https/site-relative server-side) and the source-repo
				     link is external; neither is a route resolve() can express. -->
				<a
					class="inline-flex h-12 items-center justify-center rounded-full bg-primary px-7 font-ui text-body font-medium text-white transition-colors hover:bg-primary-hover"
					href={custom ? custom.ctaHref : resolve('/login')}
					>{custom ? custom.ctaLabel : 'Create your list'}</a
				>
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
					wishes.example/sarah-home
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
										<!-- The available action mirrors the app's own wording ("Reserve it")
										     and its rose treatment, so the shot doesn't teach a word the product
										     doesn't use. -->
										<span
											class="shrink-0 rounded-full bg-primary px-2.5 py-0.5 font-ui text-chip font-medium text-white"
										>
											Reserve it
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
			<span class="font-medium text-ink"
				>{#if custom}{custom.trustLine}{:else}Free &amp; Open Source{/if}</span
			>
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

<!-- How it works: four steps, step 3 emphasised. -->
<section id="how-it-works" class="bg-page py-20">
	<div class="mx-auto max-w-7xl px-6">
		<div class="text-center">
			<span
				class="inline-block rounded-full bg-primary-tint px-3 py-1 font-ui text-chip font-semibold uppercase tracking-wide text-primary"
			>
				Simple coordination
			</span>
			<h2 class="landing-section-title mt-5 font-display text-ink-heading">
				The magic lives in the secrecy
			</h2>
			<p class="mx-auto mt-4 max-w-2xl font-ui text-[18px] leading-[1.6] text-ink-muted">
				Setting up your wishlist takes seconds, and coordinating gifts requires zero communication
				friction.
			</p>
		</div>

		<ul class="mt-14 grid gap-5 sm:grid-cols-2 lg:grid-cols-4">
			{#each steps as step (step.n)}
				<li
					class={`rounded-2xl border bg-surface p-6 ${step.featured ? 'border-primary ring-1 ring-primary' : 'border-line'}`}
				>
					<span
						class={`flex h-10 w-10 items-center justify-center rounded-lg font-display text-title font-semibold ${step.featured ? 'bg-primary text-white' : 'bg-primary-tint text-primary'}`}
						aria-hidden="true">{step.n}</span
					>
					<h3 class="mt-5 font-display text-title font-semibold text-ink-heading">{step.title}</h3>
					<p class="mt-2 font-ui text-ui leading-[1.6] text-ink-muted">{step.body}</p>
				</li>
			{/each}
		</ul>
	</div>
</section>

<!-- Built differently: three value cards on the accent ground. -->
<section id="features" class="bg-surface-accent py-20">
	<div class="mx-auto max-w-7xl px-6">
		<div class="text-center">
			<span
				class="inline-block rounded-full bg-primary-tint px-3 py-1 font-ui text-chip font-semibold uppercase tracking-wide text-primary"
			>
				Built differently
			</span>
			<h2 class="landing-section-title mt-5 font-display text-ink-heading">
				Designed for humans, not for trackers
			</h2>
			<p class="mx-auto mt-4 max-w-2xl font-ui text-[18px] leading-[1.6] text-ink-muted">
				Most wishlist platforms exist to sell your search history to advertising brokers. Yaadegar
				is a clean canvas built for pure, delightful gifting.
			</p>
		</div>

		<div class="mt-14 grid gap-5 md:grid-cols-3">
			<div class="rounded-2xl bg-surface p-7 shadow-sm">
				<span
					class="flex h-11 w-11 items-center justify-center rounded-lg bg-primary-tint text-primary"
					aria-hidden="true"
				>
					<svg
						width="22"
						height="22"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
						stroke-linecap="round"
						stroke-linejoin="round"
					>
						<circle cx="12" cy="12" r="10" /><path d="m15 9-6 6M9 9l6 6" />
					</svg>
				</span>
				<h3 class="mt-5 font-display text-title font-semibold text-ink-heading">
					Private &amp; self-hosted
				</h3>
				<!-- Deploy line names Docker Compose with SQLite or Postgres; no one-click-deploy
				     claim, since there is no such artifact. -->
				<p class="mt-2 font-ui text-ui leading-[1.6] text-ink-muted">
					Your server, your choices. Deploy with Docker Compose using SQLite or Postgres. Feel
					secure knowing nobody is packaging your intimate desires for marketing.
				</p>
			</div>
			<div class="rounded-2xl bg-surface p-7 shadow-sm">
				<span
					class="flex h-11 w-11 items-center justify-center rounded-lg bg-primary-tint text-primary"
					aria-hidden="true"
				>
					<svg
						width="22"
						height="22"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
						stroke-linecap="round"
						stroke-linejoin="round"
					>
						<path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2" /><circle cx="12" cy="7" r="4" />
					</svg>
				</span>
				<h3 class="mt-5 font-display text-title font-semibold text-ink-heading">
					Surprise stays intact
				</h3>
				<p class="mt-2 font-ui text-ui leading-[1.6] text-ink-muted">
					Because Yaadegar hides reservation statuses from the creator, you'll still gasp with
					genuine joy when you rip the wrapping paper.
				</p>
			</div>
			<div class="rounded-2xl bg-surface p-7 shadow-sm">
				<span
					class="flex h-11 w-11 items-center justify-center rounded-lg bg-primary-tint text-primary"
					aria-hidden="true"
				>
					<svg
						width="22"
						height="22"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
						stroke-linecap="round"
						stroke-linejoin="round"
					>
						<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" /><circle
							cx="9"
							cy="7"
							r="4"
						/><path d="m17 8 5 5m0-5-5 5" />
					</svg>
				</span>
				<h3 class="mt-5 font-display text-title font-semibold text-ink-heading">
					No account needed
				</h3>
				<p class="mt-2 font-ui text-ui leading-[1.6] text-ink-muted">
					Grandparents don't need to learn a new password. Family can instantly click and reserve
					from any phone without completing a sign-up flow.
				</p>
			</div>
		</div>
	</div>
</section>

<!-- Pull quote. -->
<section class="bg-page py-20">
	<figure class="mx-auto max-w-4xl px-6 text-center">
		<svg
			class="mx-auto h-10 w-10 text-primary-tint"
			viewBox="0 0 24 24"
			fill="currentColor"
			aria-hidden="true"
		>
			<path
				d="M9.5 8C7 8 5 10 5 12.5V18h5.5v-5.5H8c0-1.4 1.1-2.5 2.5-2.5V8zm9 0C16 8 14 10 14 12.5V18h5.5v-5.5H17c0-1.4 1.1-2.5 2.5-2.5V8z"
			/>
		</svg>
		<blockquote class="landing-quote mt-6 font-display text-ink-heading">
			A gift list shouldn't feel like an invoice. Yaadegar keeps the humanity and playfulness of
			gift-giving alive, wrapping the process in security.
		</blockquote>
		<figcaption class="mt-6 font-ui text-ui font-semibold uppercase tracking-wide text-ink-muted">
			Design by <a
				class="underline decoration-1 underline-offset-2 transition-colors hover:text-ink"
				href="https://github.com/mahboub8061"
				rel="noreferrer"
				target="_blank">Mahboubeh Ghafouri</a
			> · Built by AI
		</figcaption>
	</figure>
</section>

<!-- Take a look inside: a second product mockup (guest view of a shared list). Same
     replica rules as the hero — invented content, gift-glyph placeholders, app-true
     wording. -->
<section class="bg-surface-alt py-20">
	<div class="mx-auto max-w-7xl px-6">
		<div class="text-center">
			<span
				class="inline-block rounded-full bg-primary-tint px-3 py-1 font-ui text-chip font-semibold uppercase tracking-wide text-primary"
			>
				Take a look inside
			</span>
			<h2 class="landing-section-title mt-5 font-display text-ink-heading">
				Handcrafted visual elegance
			</h2>
			<!-- Copy names only what the app does today: no theming claim (the app has no
			     customizable themes yet), so "customizable cozy themes" from the design comes out. -->
			<p class="mx-auto mt-4 max-w-2xl font-ui text-[18px] leading-[1.6] text-ink-muted">
				We built the wishlist you'll actually want to share. Elegant card design, simple responsive
				layouts, and a clean, readable card for every gift.
			</p>
		</div>

		<!-- Browser-chrome replica. Invented handle/list; the URL uses the RFC-reserved
		     .example domain, like the hero, so no real address ships on a public page. -->
		<div class="mt-14 overflow-hidden rounded-2xl border border-line bg-surface-accent shadow-xl">
			<div class="flex items-center gap-2 border-b border-divider bg-surface px-4 py-3">
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
					wishes.example/cozy-winter-warmth
				</span>
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
					class="text-ink-muted"
				>
					<circle cx="18" cy="5" r="3" /><circle cx="6" cy="12" r="3" /><circle
						cx="18"
						cy="19"
						r="3"
					/><path d="m8.6 13.5 6.8 4M15.4 6.5 8.6 10.5" />
				</svg>
			</div>

			<div class="p-6 sm:p-8">
				<div class="flex flex-wrap items-end justify-between gap-4">
					<div class="min-w-0">
						<p class="flex items-center gap-2 font-ui text-ui font-medium text-primary">
							<span
								class="flex h-6 w-6 items-center justify-center rounded-full bg-primary-tint text-primary"
								aria-hidden="true"
							>
								<svg
									width="13"
									height="13"
									viewBox="0 0 24 24"
									fill="none"
									stroke="currentColor"
									stroke-width="2"
									stroke-linecap="round"
									stroke-linejoin="round"
								>
									<path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2" /><circle
										cx="12"
										cy="7"
										r="4"
									/>
								</svg>
							</span>
							@julian_reads
						</p>
						<p class="mt-1 font-display text-[30px] font-semibold leading-tight text-ink-heading">
							Cozy Winter Reading List
						</p>
					</div>
					<!-- Decorative marketing labels, not real controls: hidden from assistive tech so
					     they aren't announced as buttons a reader could press. -->
					<div class="flex shrink-0 items-center gap-3" aria-hidden="true">
						<span
							class="inline-flex h-10 items-center gap-2 rounded-full border border-line bg-surface px-4 font-ui text-ui font-medium text-ink"
						>
							<svg
								width="15"
								height="15"
								viewBox="0 0 24 24"
								fill="none"
								stroke="currentColor"
								stroke-width="2"
								stroke-linecap="round"
								stroke-linejoin="round"
							>
								<path d="M4 12v8a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-8" /><path
									d="M16 6l-4-4-4 4M12 2v13"
								/>
							</svg>
							Share
						</span>
						<span
							class="inline-flex h-10 items-center rounded-full bg-primary px-4 font-ui text-ui font-medium text-white"
						>
							Reserve secretly
						</span>
					</div>
				</div>

				<ul class="mt-6 grid gap-5 sm:grid-cols-3">
					{#each showcaseItems as item (item.name)}
						<li class="flex flex-col rounded-2xl border border-line bg-surface p-3">
							<!-- Gift-glyph placeholder on the item's accent tint — no stock photo. -->
							<div
								class={`flex aspect-[4/3] items-center justify-center rounded-xl ${item.tint} ${item.icon}`}
								aria-hidden="true"
							>
								<GiftGlyph size={40} />
							</div>
							<div class="mt-3 flex items-start justify-between gap-2">
								<h3 class="font-display text-title font-semibold leading-tight text-ink-heading">
									{item.name}
								</h3>
								<span class="shrink-0 font-display text-title font-semibold text-primary"
									>{item.price}</span
								>
							</div>
							<p class="mt-1 font-ui text-ui leading-[1.5] text-ink-muted">{item.note}</p>
							<div
								class="mt-3 flex items-center justify-between border-t border-line-subtle pt-3 font-ui text-chip"
							>
								<span class="text-ink-muted">Wants {item.wants}</span>
								{#if item.reserved}
									<span class="rounded-full bg-amber-50 px-2 py-0.5 font-medium text-amber-700"
										>Reserved</span
									>
								{:else}
									<span class="rounded-full bg-green-tint px-2 py-0.5 font-medium text-green"
										>Available</span
									>
								{/if}
							</div>
						</li>
					{/each}
				</ul>
			</div>
		</div>
	</div>
</section>

<!-- Self-hosting. Bounded by #258: an images-only compose starts the API and serves NO
     site, so the code block shows the working clone-and-build path (which builds the web
     service too and serves it on :3000), not a snippet that starts nothing a visitor sees.
     Copy states what is true today — Docker Compose, SQLite for dev / Postgres for prod —
     with no Fly.io/Railway one-click promise the repo has no artifact for. -->
<section id="self-hosting" class="bg-surface-accent py-20">
	<div class="mx-auto grid max-w-7xl gap-12 px-6 lg:grid-cols-2 lg:items-center">
		<div>
			<span
				class="inline-block rounded-full bg-primary-tint px-3 py-1 font-ui text-chip font-semibold uppercase tracking-wide text-primary"
			>
				Self-hosted
			</span>
			<h2 class="landing-section-title mt-5 font-display text-ink-heading">
				Sovereign infrastructure for sovereign celebrations
			</h2>
			<p class="mt-5 max-w-xl font-ui text-[18px] leading-[1.6] text-ink-muted">
				Yaadegar is a Go backend and a SvelteKit front end that run together under Docker Compose.
				Clone the repository, build it, and the whole stack is yours — SQLite to get started,
				PostgreSQL for production. No third-party host, no lock-in.
			</p>
			<dl class="mt-8 flex gap-8">
				<div>
					<dt class="font-display text-[26px] font-semibold text-primary">Compose</dt>
					<dd class="mt-1 font-ui text-ui text-ink-muted">One command to run</dd>
				</div>
				<div class="border-l border-line pl-8">
					<dt class="font-display text-[26px] font-semibold text-primary">SQLite</dt>
					<dd class="mt-1 font-ui text-ui text-ink-muted">or PostgreSQL for production</dd>
				</div>
			</dl>
		</div>

		<!-- The dark panel is the design's terminal; its contents are the honest, runnable
		     sequence from the README, not the design's images-only compose (#258). -->
		<div class="overflow-hidden rounded-2xl bg-ink shadow-xl">
			<div class="flex items-center gap-2 border-b border-white/10 px-4 py-3">
				<span class="flex gap-1.5" aria-hidden="true">
					<span class="h-3 w-3 rounded-full bg-primary"></span>
					<span class="h-3 w-3 rounded-full bg-gold"></span>
					<span class="h-3 w-3 rounded-full bg-green"></span>
				</span>
				<span class="mx-auto font-mono text-chip text-white/40">bash</span>
			</div>
			<pre class="overflow-x-auto px-5 py-5 font-mono text-[13px] leading-[1.7] text-white/90"><code
					><span class="text-white/40"
						># clone and build — serves the web UI on http://localhost:3000</span
					>
<span class="text-primary-hover">$</span> git clone https://github.com/yaad-index/yaadegar
<span class="text-primary-hover">$</span> cd yaadegar
<span class="text-primary-hover">$</span> docker compose up --build</code
				></pre>
		</div>
	</div>
</section>

<!-- Closing call to action. -->
<section class="bg-surface py-20">
	<div class="mx-auto max-w-3xl px-6 text-center">
		<svg
			class="mx-auto h-10 w-10 text-gold"
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			stroke-width="1.6"
			stroke-linecap="round"
			stroke-linejoin="round"
			aria-hidden="true"
		>
			<path d="M12 3l1.8 6.2L20 11l-6.2 1.8L12 19l-1.8-6.2L4 11l6.2-1.8L12 3z" /><path
				d="M19 4v3M20.5 5.5h-3"
			/>
		</svg>
		<h2 class="landing-cta-title mt-6 font-display text-ink-heading">
			Bring back the genuine delight of giving
		</h2>
		<p class="mx-auto mt-5 max-w-xl font-ui text-[18px] leading-[1.6] text-ink-muted">
			Setup Yaadegar in seconds, coordinate gifts in secrecy, and share clean wishlists with those
			you love. Absolutely free, forever open source.
		</p>
		<div class="mt-8 flex flex-wrap items-center justify-center gap-3">
			<a
				class="inline-flex h-12 items-center justify-center rounded-full bg-primary px-7 font-ui text-body font-medium text-white transition-colors hover:bg-primary-hover"
				href={resolve('/login')}>Create your wishlist</a
			>
			<!-- eslint-disable svelte/no-navigation-without-resolve -- same-page anchor to the
			     self-hosting section, which resolve() can't express. -->
			<a
				class="inline-flex h-12 items-center justify-center rounded-full border border-primary bg-surface px-6 font-ui text-body font-medium text-primary transition-colors hover:bg-primary-tint"
				href="#self-hosting">Deploy self-hosted</a
			>
			<!-- eslint-enable svelte/no-navigation-without-resolve -->
		</div>
	</div>
</section>

<!-- Footer: wordmark + two link columns, all pointing at resources that exist today. -->
<footer class="border-t border-divider bg-surface-alt">
	<div class="mx-auto max-w-7xl px-6 py-14">
		<div class="flex flex-col gap-10 md:flex-row md:justify-between">
			<div class="max-w-sm">
				<p class="flex items-center gap-2 font-display text-display-sm font-semibold text-primary">
					<svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
						<path d="M12 3l1.8 6.2L20 11l-6.2 1.8L12 19l-1.8-6.2L4 11l6.2-1.8L12 3z" />
					</svg>
					Yaadegar
				</p>
				<p class="mt-3 font-ui text-ui leading-[1.6] text-ink-muted">
					The cozy, open-source gift-list manager built to protect surprise and personal privacy.
				</p>
			</div>

			<!-- eslint-disable svelte/no-navigation-without-resolve -- external links to the source
			     repository and its GitHub resources; none is a SvelteKit route resolve() can express. -->
			<div class="flex gap-16">
				{#each footerColumns as col (col.heading)}
					<div>
						<p class="font-ui text-chip font-semibold uppercase tracking-wide text-ink-heading">
							{col.heading}
						</p>
						<ul class="mt-4 space-y-3 font-ui text-ui text-ink-muted">
							{#each col.links as link (link.label)}
								<li>
									<a
										class="transition-colors hover:text-ink"
										href={link.href}
										rel="noreferrer"
										target="_blank">{link.label}</a
									>
								</li>
							{/each}
						</ul>
					</div>
				{/each}
			</div>
			<!-- eslint-enable svelte/no-navigation-without-resolve -->
		</div>

		<div
			class="mt-12 flex flex-col gap-2 border-t border-divider pt-6 font-ui text-chip text-ink-muted sm:flex-row sm:justify-between"
		>
			<span>© 2026 Yaadegar Project · Released under the MIT License.</span>
			<span>Built with love by AI agents for sovereign gift-giving.</span>
		</div>
	</div>
</footer>

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

	/* Landing-local section heading: 40px/600. The app's shared 40px display role
	   (.display-title) is 700 weight, so this lighter cut lives here, not on that role. */
	.landing-section-title {
		font-size: 40px;
		font-weight: 600;
		line-height: 1.15;
	}
	@media (max-width: 640px) {
		.landing-section-title {
			font-size: 30px;
		}
	}

	/* Landing-local closing-CTA type: 48px/600, a display size used only here. It sits
	   between the section titles (40px) and the hero (54px); measured against the export
	   by line ink-extent (~47px) and stem width (matches the 600 cut, not the 700 hero).
	   Landing-local like the rest of this scale — it does NOT touch the shared app rungs,
	   which is the type-scale decision left open on #236. */
	.landing-cta-title {
		font-size: 48px;
		font-weight: 600;
		line-height: 1.15;
	}
	@media (max-width: 640px) {
		.landing-cta-title {
			font-size: 34px;
		}
	}

	/* Landing-local pull-quote type: 35px/500, a display size used only here. */
	.landing-quote {
		font-size: 35px;
		font-weight: 500;
		line-height: 1.35;
	}
	@media (max-width: 640px) {
		.landing-quote {
			font-size: 24px;
		}
	}
</style>
