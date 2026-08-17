<script lang="ts">
	// A password field with a show/hide visibility toggle (#201). Same control
	// treatment and announced-error wiring as Field, plus a toggle button on the
	// right that flips the input between password and text. `value` is bindable and
	// all native input attributes (name, autocomplete, minlength, required, …) pass
	// through, so it drops into the existing forms without changing their contract.
	import type { HTMLInputAttributes } from 'svelte/elements';

	interface Props extends HTMLInputAttributes {
		label: string;
		error?: string;
		hint?: string;
		value?: string;
	}

	let { label, error, hint, value = $bindable(), id, class: klass = '', ...rest }: Props = $props();

	let visible = $state(false);

	const uid = $props.id();
	const inputId = $derived(id ?? `${uid}-input`);
	const errorId = `${uid}-error`;
	const hintId = `${uid}-hint`;

	const invalid = $derived(Boolean(error));
	const describedBy = $derived(error ? errorId : hint ? hintId : undefined);
</script>

<div class="block">
	<label for={inputId} class="mb-1 block font-ui text-ui font-medium text-ink">{label}</label>
	<div class="relative">
		<input
			{...rest}
			id={inputId}
			type={visible ? 'text' : 'password'}
			bind:value
			aria-invalid={invalid || undefined}
			aria-describedby={describedBy}
			class={`h-12 w-full rounded-card border bg-surface-alt pl-3 pr-12 font-ui text-body text-ink placeholder:text-ink-muted focus:outline-none focus-visible:ring-2 ${
				invalid
					? 'border-red-500 focus-visible:ring-red-500'
					: 'border-line-subtle focus-visible:border-primary focus-visible:ring-primary'
			} ${klass}`}
		/>
		<button
			type="button"
			onclick={() => (visible = !visible)}
			aria-pressed={visible}
			aria-label={visible ? 'Hide password' : 'Show password'}
			class="absolute inset-y-0 right-0 flex items-center rounded-r-card px-3 text-ink-muted hover:text-ink focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
		>
			{#if visible}
				<!-- eye-off -->
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
					<path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 10 8 10 8a18.5 18.5 0 0 1-2.16 3.19" />
					<path d="M6.61 6.61A18.5 18.5 0 0 0 2 12s3 8 10 8a9.12 9.12 0 0 0 5.39-1.61" />
					<path d="M14.12 14.12A3 3 0 1 1 9.88 9.88" />
					<line x1="2" y1="2" x2="22" y2="22" />
				</svg>
			{:else}
				<!-- eye -->
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
					<path d="M2 12s3-8 10-8 10 8 10 8-3 8-10 8-10-8-10-8Z" />
					<circle cx="12" cy="12" r="3" />
				</svg>
			{/if}
		</button>
	</div>
	{#if error}
		<p id={errorId} class="mt-1 font-ui text-ui text-red-600" role="alert">{error}</p>
	{:else if hint}
		<p id={hintId} class="mt-1 font-ui text-ui text-ink-muted">{hint}</p>
	{/if}
</div>
