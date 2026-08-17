<script lang="ts">
	// The signed-in account actions in the top bar (#205/#206): a muted identity,
	// a divider, then the weighted action group with the current route marked
	// active. Emphasis is deliberate — the design mutes who you are and weights
	// what you can do, and marks where you are (functional, not cosmetic). Takes
	// the current pathname as a prop so the active-route logic stays testable.
	import { resolve } from '$app/paths';

	interface Props {
		name: string;
		isAdmin: boolean;
		/** The current route path (page.url.pathname), for active marking. */
		pathname: string;
	}

	let { name, isAdmin, pathname }: Props = $props();

	type NavLink = { label: string; href: string };
	const links: NavLink[] = $derived([
		{ label: 'Reserved', href: resolve('/(app)/reservations') },
		...(isAdmin ? [{ label: 'Admin', href: resolve('/admin') }] : []),
		{ label: 'Settings', href: resolve('/(app)/settings') }
	]);

	const isActive = (href: string) => pathname === href || pathname.startsWith(href + '/');
</script>

<div class="flex items-center gap-4">
	<!-- Identity: muted and lighter — the design de-emphasises who you are. -->
	<span class="font-normal text-ink-muted">{name}</span>
	<!-- Divider so the identity and the action group read as two clusters. -->
	<span class="h-4 w-px bg-line" aria-hidden="true"></span>
	<!--
		Active destination: rose text + a rose dot (both, per the export). The dot
		marks a destination you are on; "Log out" is an action, not a destination,
		so it never carries one. Inactive items get no dot — the one restrained
		inactive treatment; the export's inactive-dot rule is inconsistent across
		screens and is handed to the designer rather than invented here.
	-->
	{#each links as link (link.href)}
		{@const active = isActive(link.href)}
		<!-- eslint-disable svelte/no-navigation-without-resolve -- href is a resolve()'d path held in the links array. -->
		<a
			href={link.href}
			aria-current={active ? 'page' : undefined}
			class={`inline-flex items-center gap-1.5 font-medium transition-colors ${
				active ? 'text-primary' : 'text-ink hover:text-primary-hover'
			}`}
		>
			{#if active}<span class="h-1.5 w-1.5 rounded-full bg-primary" aria-hidden="true"></span>{/if}
			{link.label}
		</a>
		<!-- eslint-enable svelte/no-navigation-without-resolve -->
	{/each}
	<form method="post" action="/logout">
		<button class="font-medium text-ink transition-colors hover:text-primary-hover" type="submit">
			Log out
		</button>
	</form>
</div>
