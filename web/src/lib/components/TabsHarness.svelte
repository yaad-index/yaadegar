<script lang="ts">
	// Test fixture mirroring how the list-detail page composes <Tabs> (#128): the tab
	// bar drives a local `active` state, the visible panel keys off it, and a draft
	// input is bound to persistent state (as the real page's add-item fields are bound
	// to the superForm store) so a tab switch does not lose in-progress input.
	import Tabs from './Tabs.svelte';

	let { initial = 'list' }: { initial?: string } = $props();

	// Intentionally captures only the initial prop (like the page derives its first
	// tab from the URL once); clicks drive it thereafter.
	// svelte-ignore state_referenced_locally
	let active = $state(initial);
	let selected = $state<string[]>([]);
	let draft = $state('');

	const tabs = [
		{ id: 'list', label: 'List', href: '/l?tab=list' },
		{ id: 'settings', label: 'Settings', href: '/l?tab=settings' }
	];
</script>

<Tabs {tabs} bind:active onselect={(id) => (selected = [...selected, id])} />

{#if active === 'settings'}
	<div data-testid="panel">settings-panel</div>
{:else}
	<div data-testid="panel">
		list-panel
		<input aria-label="draft" bind:value={draft} />
	</div>
{/if}

<div data-testid="selected">{selected.join(',')}</div>
