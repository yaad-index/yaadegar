<script lang="ts">
	// A form field with its label, help text and error states (#199) — the single
	// treatment every screen's inputs share. The control is the 48px height shared
	// with Button. `value` is bindable so it drops into existing forms unchanged;
	// all native input attributes (name, type, autocomplete, required, …) pass
	// through. Label/error/hint are wired with matching id/for/aria-describedby so
	// the error is announced, not just coloured.
	import type { HTMLInputAttributes } from 'svelte/elements';

	interface Props extends HTMLInputAttributes {
		label: string;
		/** When set, the field renders its error state and announces the message. */
		error?: string;
		/** Helper text shown only while there is no error. */
		hint?: string;
		value?: string;
	}

	let { label, error, hint, value = $bindable(), id, class: klass = '', ...rest }: Props = $props();

	// SSR-stable ids so label association survives hydration without a mismatch.
	const uid = $props.id();
	const inputId = $derived(id ?? `${uid}-input`);
	const errorId = `${uid}-error`;
	const hintId = `${uid}-hint`;

	const invalid = $derived(Boolean(error));
	const describedBy = $derived(error ? errorId : hint ? hintId : undefined);
</script>

<div class="block">
	<label for={inputId} class="mb-1 block font-ui text-ui font-medium text-ink">{label}</label>
	<input
		{...rest}
		id={inputId}
		bind:value
		aria-invalid={invalid || undefined}
		aria-describedby={describedBy}
		class={`h-12 w-full rounded-card border bg-surface-alt px-3 font-ui text-body text-ink placeholder:text-ink-muted focus:outline-none ${
			invalid ? 'border-red-500 focus:border-red-500' : 'border-line-subtle focus:border-primary'
		} ${klass}`}
	/>
	{#if error}
		<p id={errorId} class="mt-1 font-ui text-ui text-red-600" role="alert">{error}</p>
	{:else if hint}
		<p id={hintId} class="mt-1 font-ui text-ui text-ink-muted">{hint}</p>
	{/if}
</div>
