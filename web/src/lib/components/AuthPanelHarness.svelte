<script lang="ts">
	// Test harness: gives AuthPanel its description, children, and links snippets.
	import AuthPanel from './AuthPanel.svelte';
</script>

<AuthPanel heading="Sign in">
	{#snippet description()}Welcome back{/snippet}
	<p data-testid="form-body">the form</p>
	{#snippet links()}
		<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- static href in a test harness -->
		<a href="/forgot" data-testid="forgot-link">Forgot your password?</a>
	{/snippet}
</AuthPanel>
