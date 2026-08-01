<script lang="ts">
	// A self-contained, reactive tab bar (#128). The active tab is local reactive
	// state set directly on click, so the selection — and any panel the parent keys
	// off `active` — switches immediately, without depending on the URL re-deriving
	// (the bug the earlier version had: relying on a shallow-routing URL update to
	// re-fire a derived value, which it does not). Each tab carries a real `href`, so
	// a no-JS click is a normal navigation; with JS the click is intercepted for an
	// in-place switch, and `onselect` lets the parent mirror the choice elsewhere
	// (e.g. into the URL for deep-linking / reload persistence).
	type Tab = { id: string; label: string; href: string };

	let {
		tabs,
		active = $bindable(),
		onselect,
		label = 'Sections'
	}: {
		tabs: Tab[];
		active: string;
		onselect?: (id: string) => void;
		label?: string;
	} = $props();

	function select(event: MouseEvent, id: string) {
		event.preventDefault();
		active = id;
		onselect?.(id);
	}
</script>

<nav class="mt-4 flex gap-1 border-b text-sm" aria-label={label}>
	{#each tabs as tab (tab.id)}
		<!-- eslint-disable svelte/no-navigation-without-resolve -- caller-supplied internal
		     href (query-only, the no-JS fallback); JS intercepts for an in-place switch. -->
		<a
			href={tab.href}
			onclick={(e) => select(e, tab.id)}
			aria-current={active === tab.id ? 'page' : undefined}
			class={`-mb-px border-b-2 px-3 py-2 ${
				active === tab.id
					? 'border-black font-medium'
					: 'border-transparent text-gray-500 hover:text-gray-700'
			}`}
		>
			{tab.label}
		</a>
		<!-- eslint-enable svelte/no-navigation-without-resolve -->
	{/each}
</nav>
