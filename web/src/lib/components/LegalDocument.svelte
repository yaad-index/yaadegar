<script lang="ts">
	// The shared body for /privacy and /terms (#300): a plain prose column on the page
	// background, with the wordmark linking home. Both routes render this; only the
	// document differs.
	//
	// The installation's text is interpolated, so Svelte escapes it — operator wording is
	// content, never markup. `whitespace-pre-line` keeps the line breaks inside a paragraph
	// that the operator wrote, while blank lines have already become separate paragraphs
	// (lib/legal.ts).
	import { resolve } from '$app/paths';
	import { PLACEHOLDER_MARKER, type LegalPage } from '$lib/legal';

	let { page }: { page: LegalPage } = $props();

	// What the placeholder says this installation has not published. Naming the document
	// rather than saying "this page" keeps the sentence true when read on its own.
	const noun = $derived(page.doc === 'privacy' ? 'a privacy policy' : 'terms of service');
</script>

<svelte:head><title>{page.title} · Yaadegar</title></svelte:head>

<main class="flex-1 bg-page px-6 py-16">
	<div class="mx-auto max-w-3xl">
		<a
			class="font-display text-ui font-semibold tracking-[0.2em] text-primary transition-colors hover:text-primary-hover"
			href={resolve('/')}>YAADEGAR</a
		>

		<h1 class="mt-6 font-display text-display-sm font-semibold text-ink-heading">{page.title}</h1>

		{#if page.configured}
			{#each page.paragraphs as paragraph, i (i)}
				<p class="mt-5 whitespace-pre-line font-ui text-body leading-[1.7] text-ink">
					{paragraph}
				</p>
			{/each}
		{:else}
			<!-- No default legal wording ships with the project (see lib/legal.ts): an
			     unconfigured instance says plainly that it has published nothing. -->
			<p
				class="mt-6 font-ui text-title font-semibold uppercase tracking-wide text-ink-heading"
				data-testid="legal-placeholder"
			>
				{PLACEHOLDER_MARKER}
			</p>
			<p class="mt-4 font-ui text-body leading-[1.7] text-ink-muted">
				This installation has not published {noun}. Yaadegar ships no default wording, because the
				text belongs to whoever runs this instance and to their jurisdiction.
			</p>
			<p class="mt-4 font-ui text-ui leading-[1.7] text-ink-muted">
				Running this instance? Set <code class="font-mono">{page.envKey}</code> on the web service to
				replace this page with your own text.
			</p>
		{/if}
	</div>
</main>
