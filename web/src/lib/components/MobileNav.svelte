<script lang="ts">
	// The mobile signed-in navigation (#229): a hamburger in the collapsed header
	// (< sm) and a right-side drawer with a scrim. Desktop keeps the inline
	// AccountNav (rendered alongside this, `hidden sm:flex`); this whole component
	// is `sm:hidden`, so the two never show at once. The drawer lists the same
	// destinations as the desktop nav plus "My lists" (the home the wordmark links
	// to on desktop), as icon+label rows.
	//
	// Active-row treatment is GREEN here (accent-green tint + text), matching the
	// mobile export — the desktop nav marks active in ROSE. That cross-surface
	// divergence is the design's, recorded rather than reconciled (measured, not
	// invented): reconciling it here would destroy the evidence it exists.
	import { resolve } from '$app/paths';
	import { afterNavigate } from '$app/navigation';

	interface Props {
		/** The signed-in identity, muted. This is the account name; the backend
		    defaults it to the email when no display name is set, which is what the
		    export's "sara@email.com" is. /me returns name (not a separate email), and
		    the desktop nav shows the same field, so both surfaces stay consistent. */
		name: string;
		isAdmin: boolean;
		/** Current route path (page.url.pathname), for active marking. */
		pathname: string;
	}

	let { name, isAdmin, pathname }: Props = $props();

	let open = $state(false);
	let closeButton = $state<HTMLButtonElement | null>(null);
	let hamburger = $state<HTMLButtonElement | null>(null);
	let panel = $state<HTMLElement | null>(null);

	const home = resolve('/(app)/lists');
	type Row = { label: string; href: string; icon: 'gift' | 'lock' | 'shield' | 'gear' };
	const rows: Row[] = $derived([
		{ label: 'My lists', href: home, icon: 'gift' },
		{ label: 'Reserved', href: resolve('/(app)/reservations'), icon: 'lock' },
		// The Admin row's icon is a CHOICE, not a reading — the export's mock user is
		// not an admin, so the design does not show it (a shield is the convention).
		...(isAdmin ? [{ label: 'Admin', href: resolve('/admin'), icon: 'shield' } as Row] : []),
		{ label: 'Settings', href: resolve('/(app)/settings'), icon: 'gear' }
	]);

	// Home is active only on the exact root; every path startsWith('/') so the
	// prefix rule can't apply to it.
	const isActive = (href: string) =>
		href === home ? pathname === home : pathname === href || pathname.startsWith(href + '/');

	// A navigation closes the drawer (a row was tapped, or any route change). No
	// focus restore here — the destination page owns focus after a navigation.
	afterNavigate(() => (open = false));

	// Close and hand focus back to the trigger (Escape / scrim / close button).
	function close() {
		open = false;
		hamburger?.focus();
	}

	// Lock body scroll while the drawer is open; focus the close control on open.
	$effect(() => {
		if (typeof document === 'undefined') return;
		document.body.style.overflow = open ? 'hidden' : '';
		if (open) closeButton?.focus();
		return () => {
			document.body.style.overflow = '';
		};
	});

	// While open: close on Escape, and keep Tab focus inside the drawer. The panel
	// sits before <main> in the DOM, so without a trap Tab would leave the dialog
	// into the scrimmed page (there is no `inert` helper in this codebase).
	function onKeydown(e: KeyboardEvent) {
		if (!open) return;
		if (e.key === 'Escape') {
			close();
			return;
		}
		if (e.key !== 'Tab' || !panel) return;
		const focusable = panel.querySelectorAll<HTMLElement>(
			'a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])'
		);
		if (focusable.length === 0) return;
		const first = focusable[0];
		const last = focusable[focusable.length - 1];
		const active = document.activeElement;
		if (e.shiftKey) {
			if (active === first || !panel.contains(active)) {
				last.focus();
				e.preventDefault();
			}
		} else if (active === last || !panel.contains(active)) {
			first.focus();
			e.preventDefault();
		}
	}
</script>

<svelte:window onkeydown={onKeydown} />

<!-- Hamburger: 18x12, right-aligned in the bar, only on the collapsed header. -->
<button
	bind:this={hamburger}
	type="button"
	class="text-ink transition-colors hover:text-primary sm:hidden"
	aria-label="Open menu"
	aria-expanded={open}
	onclick={() => (open = true)}
>
	<svg
		width="18"
		height="12"
		viewBox="0 0 18 12"
		fill="none"
		stroke="currentColor"
		stroke-width="2"
		stroke-linecap="round"
		aria-hidden="true"
	>
		<path d="M0 1h18M0 6h18M0 11h18" />
	</svg>
</button>

{#if open}
	<!-- Overlay: scrim (40% black) + the right-side panel. Fixed, above content,
	     mobile only. Clicking the scrim closes; the panel stops propagation. -->
	<div class="fixed inset-0 z-50 sm:hidden">
		<!-- Scrim: a mouse convenience for closing, hidden from assistive tech so it is
		     not a duplicate "Close menu" in the controls list — the labelled close
		     button and Escape own the accessible close. -->
		<button
			type="button"
			class="absolute inset-0 h-full w-full cursor-default bg-black/40"
			aria-hidden="true"
			tabindex="-1"
			onclick={close}
		></button>
		<div
			bind:this={panel}
			class="absolute right-0 top-0 flex h-full w-[74%] max-w-[289px] flex-col border-l border-line bg-surface"
			role="dialog"
			aria-modal="true"
			aria-label="Menu"
		>
			<!-- Close control top-left. Keep the design's panel-collapse glyph; the
			     accessible name lives in aria-label, not the icon. -->
			<div class="px-4 pb-2 pt-4">
				<button
					bind:this={closeButton}
					type="button"
					class="text-ink transition-colors hover:text-primary"
					aria-label="Close menu"
					onclick={close}
				>
					<svg
						width="22"
						height="16"
						viewBox="0 0 22 16"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
						stroke-linecap="round"
						stroke-linejoin="round"
						aria-hidden="true"
					>
						<path d="M21 3H9M21 8H9M21 13H9" />
						<path d="M5 4 1 8l4 4" />
					</svg>
				</button>
			</div>

			<!-- Signed-in identity, muted (the account name; defaults to the email
			     when no display name is set, as on the desktop nav). -->
			<p class="truncate px-4 pb-3 font-ui text-ui text-ink-muted">{name}</p>
			<div class="border-b border-line"></div>

			<!-- Destination rows + Log out. The active row is tinted (accent-green). -->
			<nav class="flex flex-col gap-1 p-3">
				{#each rows as row (row.href)}
					{@const active = isActive(row.href)}
					<!-- eslint-disable svelte/no-navigation-without-resolve -- href is a resolve()'d path. -->
					<a
						href={row.href}
						aria-current={active ? 'page' : undefined}
						class={`flex items-center gap-3 rounded-card px-3 py-2.5 font-ui text-body font-medium transition-colors ${
							active ? 'bg-green-tint text-green' : 'text-ink hover:bg-surface-alt'
						}`}
					>
						{#if row.icon === 'gift'}
							<svg
								class="shrink-0"
								width="20"
								height="20"
								viewBox="0 0 24 24"
								fill="none"
								stroke="currentColor"
								stroke-width="2"
								stroke-linecap="round"
								stroke-linejoin="round"
								aria-hidden="true"
								><rect x="3" y="8" width="18" height="4" rx="1" /><path
									d="M12 8v13M5 12v7a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-7"
								/><path d="M12 8S9.5 3.5 7.5 4.5 7 8 12 8Zm0 0s2.5-4.5 4.5-3.5S17 8 12 8Z" /></svg
							>
						{:else if row.icon === 'lock'}
							<svg
								class="shrink-0"
								width="20"
								height="20"
								viewBox="0 0 24 24"
								fill="none"
								stroke="currentColor"
								stroke-width="2"
								stroke-linecap="round"
								stroke-linejoin="round"
								aria-hidden="true"
								><rect x="3" y="11" width="18" height="11" rx="2" /><path
									d="M7 11V7a5 5 0 0 1 10 0v4"
								/></svg
							>
						{:else if row.icon === 'shield'}
							<svg
								class="shrink-0"
								width="20"
								height="20"
								viewBox="0 0 24 24"
								fill="none"
								stroke="currentColor"
								stroke-width="2"
								stroke-linecap="round"
								stroke-linejoin="round"
								aria-hidden="true"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10Z" /></svg
							>
						{:else}
							<svg
								class="shrink-0"
								width="20"
								height="20"
								viewBox="0 0 24 24"
								fill="none"
								stroke="currentColor"
								stroke-width="2"
								stroke-linecap="round"
								stroke-linejoin="round"
								aria-hidden="true"
								><circle cx="12" cy="12" r="3" /><path
									d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 8 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H2a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 3.6 8a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H8a1.65 1.65 0 0 0 1-1.51V2a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V8a1.65 1.65 0 0 0 1.51 1H22a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1Z"
								/></svg
							>
						{/if}
						{row.label}
					</a>
					<!-- eslint-enable svelte/no-navigation-without-resolve -->
				{/each}
				<form method="post" action="/logout">
					<button
						type="submit"
						class="flex w-full items-center gap-3 rounded-card px-3 py-2.5 font-ui text-body font-medium text-ink transition-colors hover:bg-surface-alt"
					>
						<svg
							class="shrink-0"
							width="20"
							height="20"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="2"
							stroke-linecap="round"
							stroke-linejoin="round"
							aria-hidden="true"
							><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" /><path
								d="M16 17l5-5-5-5M21 12H9"
							/></svg
						>
						Log out
					</button>
				</form>
			</nav>
		</div>
	</div>
{/if}
