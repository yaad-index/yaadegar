<script lang="ts">
	// Site-level legal links for every page that is not the landing page (#302).
	//
	// /privacy and /terms exist on every instance (#300), but the only links to them
	// lived in the landing footer. An instance running YAADEGAR_ROOT_PAGE=login never
	// renders that page, so the routes resolved and nothing pointed at them: a visitor
	// had to already know the URLs. Self-registration is the case that makes this
	// matter — an account gets created, and the terms it agrees to are unreachable.
	//
	// Labels match the landing footer's Legal column exactly, so the same two documents
	// are not named two different things depending on where they are reached from.
	import { resolve } from '$app/paths';
</script>

<footer class="border-t border-divider px-6 py-6">
	<nav
		aria-label="Legal"
		class="mx-auto flex max-w-7xl flex-wrap justify-center gap-x-6 gap-y-2 font-ui text-ui text-ink-muted"
	>
		<a class="transition-colors hover:text-ink" href={resolve('/privacy')}>Privacy Policy</a>
		<a class="transition-colors hover:text-ink" href={resolve('/terms')}>Terms of Service</a>
	</nav>
</footer>
