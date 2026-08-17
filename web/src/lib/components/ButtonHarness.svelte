<script lang="ts">
	// Test harness: gives Button its required children snippet.
	import Button from './Button.svelte';

	let {
		variant = 'primary',
		disabled = false,
		type = 'button'
	}: {
		variant?: 'primary' | 'secondary';
		disabled?: boolean;
		type?: 'button' | 'submit';
	} = $props();
</script>

<Button {variant} {disabled} {type}>Save</Button>
