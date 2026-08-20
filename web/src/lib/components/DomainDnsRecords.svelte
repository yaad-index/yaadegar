<script lang="ts">
	// The DNS records an owner publishes to verify a custom domain (CNAME + TXT).
	// When the instance has no CNAME target configured the value would be blank —
	// which reads as a value the owner failed to load, so they'd publish an empty
	// record. Render an explicit "not configured on this instance" note instead: it
	// is the instance's missing config, not the owner's mistake, and they cannot set
	// it from this screen. Verification is gated on the same condition (see the
	// Settings page's Verify button). Extracted from the Settings page so the
	// configured/not-configured branch is unit-tested (#239).
	import { txtRecordName } from '$lib/domains';

	interface Props {
		hostname?: string | null;
		cnameTarget?: string | null;
		verificationToken?: string | null;
	}

	let { hostname, cnameTarget, verificationToken }: Props = $props();

	const cnameConfigured = $derived(Boolean(cnameTarget));
</script>

<dl class="space-y-2 rounded-card border border-line bg-surface p-3 text-chip">
	<div>
		<dt class="font-mono text-ink-muted">CNAME · {hostname}</dt>
		{#if cnameConfigured}
			<dd class="break-all font-mono text-ink">{cnameTarget}</dd>
		{:else}
			<dd class="font-ui text-ink-muted" data-testid="cname-not-configured">
				This instance has no CNAME target configured, so custom domains can't be served yet — the
				operator needs to set one.
			</dd>
		{/if}
	</div>
	<div>
		<dt class="font-mono text-ink-muted">TXT · {txtRecordName(hostname ?? '')}</dt>
		<dd class="break-all font-mono text-ink">{verificationToken}</dd>
	</div>
</dl>
