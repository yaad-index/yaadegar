<script lang="ts">
	// The button family (#199): one 48px-tall control shared with inputs, a rose
	// primary and a bordered secondary. All native button attributes pass through,
	// so callers keep type="submit", disabled, formaction, aria-*, onclick, etc.
	import type { Snippet } from 'svelte';
	import type { HTMLButtonAttributes } from 'svelte/elements';

	type Variant = 'primary' | 'secondary';

	interface Props extends HTMLButtonAttributes {
		variant?: Variant;
		/** Stretch to the full width of the form/column (the primary CTA does this). */
		full?: boolean;
		children: Snippet;
	}

	let {
		variant = 'primary',
		full = false,
		type = 'button',
		class: klass = '',
		children,
		...rest
	}: Props = $props();

	const base =
		'inline-flex h-12 items-center justify-center rounded-card px-6 font-ui text-ui font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary disabled:cursor-not-allowed disabled:opacity-50';

	const variants: Record<Variant, string> = {
		primary: 'bg-primary text-white hover:bg-primary-hover',
		secondary: 'border border-line bg-surface text-ink hover:bg-surface-alt'
	};
</script>

<button {type} class={`${base} ${variants[variant]} ${full ? 'w-full' : ''} ${klass}`} {...rest}>
	{@render children()}
</button>
