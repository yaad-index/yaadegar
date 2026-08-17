<script module lang="ts">
	// The product's core promise (#199), single-sourced. It is deliberate product
	// copy, not sample text, so the wording lives in one place and can be imported
	// where the raw string is needed (e.g. a page <title> or a test).
	export const RESERVATION_PRIVACY_PROMISE = 'The list owner never learns who reserved';
</script>

<!--
  On the reservations screen this sits directly under the heading. Rendering it
  from a component keeps the wording from drifting into per-screen sample copy.
-->
<p class="font-ui text-ui text-ink-muted">{RESERVATION_PRIVACY_PROMISE}</p>
