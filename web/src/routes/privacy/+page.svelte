<script lang="ts">
	import LegalDocument from '$lib/components/LegalDocument.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
</script>

<LegalDocument page={data.page} />
