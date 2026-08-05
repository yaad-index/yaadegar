<script module lang="ts">
	// Provider wiring for the giver captcha widget (ADR-0013). Two shapes share this
	// component: the managed token providers (Turnstile / hCaptcha / reCAPTCHA), which
	// load a vendor SDK and render with a public site key, and Altcha (cut 2), a
	// self-hosted proof-of-work widget that needs no site key and pulls a server-issued
	// challenge from our own origin — no third party at all.
	interface CaptchaApi {
		render(
			el: HTMLElement,
			opts: {
				sitekey: string;
				callback: (token: string) => void;
				'expired-callback'?: () => void;
				'error-callback'?: () => void;
			}
		): unknown;
		ready?(cb: () => void): void;
	}

	interface ProviderConfig {
		src: string;
		global: string;
		// Whether to gate render() behind the SDK's ready() callback. Only reCAPTCHA
		// needs it. Turnstile explicitly FORBIDS ready() when its api.js is loaded
		// async/defer (it logs "Remove async/defer ... before using turnstile.ready()"
		// and never fires the callback), so for Turnstile/hCaptcha we render directly on
		// the script's onload, when the global is already usable.
		usesReady: boolean;
	}

	// The managed token providers only. Exported so tests can assert the URL/global/
	// ready mapping without a live network. Altcha is not here — it has no SDK URL.
	export const captchaProviders: Record<string, ProviderConfig> = {
		turnstile: {
			src: 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit',
			global: 'turnstile',
			usesReady: false
		},
		hcaptcha: {
			src: 'https://js.hcaptcha.com/1/api.js?render=explicit',
			global: 'hcaptcha',
			usesReady: false
		},
		recaptcha: {
			src: 'https://www.google.com/recaptcha/api.js?render=explicit',
			global: 'grecaptcha',
			usesReady: true
		}
	};

	// The self-hosted proof-of-work provider (ADR-0013 cut 2). The widget is bundled
	// with the app (no CDN) and fetches its challenge from this same-origin path, which
	// the /api/v1 proxy forwards to the backend's GET /api/v1/public/captcha/challenge.
	export const ALTCHA_PROVIDER = 'altcha';
	export const ALTCHA_CHALLENGE_URL = '/api/v1/public/captcha/challenge';
</script>

<script lang="ts">
	import { browser } from '$app/environment';

	// provider selects the widget; siteKey is the public render key (managed providers
	// only — Altcha needs none). Both come from the instance auth-methods config. The
	// parent only mounts this when provider is a known captcha-enabled value.
	let { provider, siteKey }: { provider: string; siteKey: string } = $props();

	// The resolved challenge token, surfaced to the enclosing <form> through a hidden
	// input named captcha_token (the field the reserve action reads).
	let token = $state('');
	let container = $state<HTMLDivElement | null>(null);

	const managedConfig = $derived(captchaProviders[provider]);
	const isAltcha = $derived(provider === ALTCHA_PROVIDER);
	// Whether to render at all: a managed provider needs a site key; Altcha needs
	// neither a site key nor an SDK URL (self-hosted proof-of-work).
	const active = $derived(isAltcha || (!!managedConfig && !!siteKey));

	function apiFor(name: string): CaptchaApi | undefined {
		return (window as unknown as Record<string, CaptchaApi | undefined>)[name];
	}

	// Load a managed provider SDK once (deduped by a stable id), then explicit-render
	// into our container with a callback that stores the token. Only reCAPTCHA gates
	// render on grecaptcha.ready(); Turnstile/hCaptcha render directly on onload
	// (Turnstile forbids ready() under async/defer — see ProviderConfig.usesReady).
	function renderWidget(cfg: ProviderConfig, el: HTMLElement) {
		const doRender = () => {
			const api = apiFor(cfg.global);
			if (!api) return;
			api.render(el, {
				sitekey: siteKey,
				callback: (t: string) => (token = t),
				'expired-callback': () => (token = ''),
				'error-callback': () => (token = '')
			});
		};
		const start = () => {
			const api = apiFor(cfg.global);
			if (cfg.usesReady && api?.ready) api.ready(doRender);
			else doRender();
		};

		const id = `captcha-sdk-${cfg.global}`;
		if (document.getElementById(id)) {
			start();
			return;
		}
		const script = document.createElement('script');
		script.id = id;
		script.src = cfg.src;
		script.async = true;
		script.defer = true;
		script.onload = start;
		document.head.appendChild(script);
	}

	// Render the Altcha proof-of-work widget. The library is imported dynamically so it
	// is code-split into its own chunk — only instances actually using Altcha ever load
	// it, and it never reaches a managed/disabled instance. Importing registers the
	// <altcha-widget> custom element; we point it at our same-origin challenge endpoint
	// and copy the solved payload into the token on the `verified` state. On any other
	// state (error/expired/re-solving) the token clears, so the gate fails closed.
	async function renderAltcha(el: HTMLElement) {
		if (el.querySelector('altcha-widget')) return;
		try {
			await import('altcha'); // vendored/bundled — no CDN
			const widget = document.createElement('altcha-widget');
			// The `challenge` config accepts a URL to fetch the challenge from (altcha
			// v3), NOT a `challengeurl` attribute. `auto="onload"` solves the
			// proof-of-work as soon as the widget mounts, so the token is ready by the
			// time the giver submits the reserve.
			widget.setAttribute('challenge', ALTCHA_CHALLENGE_URL);
			widget.setAttribute('auto', 'onload');
			widget.addEventListener('statechange', (e) => {
				const detail = (e as CustomEvent<{ payload?: string; state: string }>).detail;
				token = detail?.state === 'verified' && detail.payload ? detail.payload : '';
			});
			el.appendChild(widget);
		} catch {
			// Could not initialize the widget; leave the token empty so the low-trust
			// reserve fails closed rather than silently proceeding unguarded.
		}
	}

	$effect(() => {
		if (!browser || !container) return;
		if (isAltcha) {
			renderAltcha(container);
			return;
		}
		if (managedConfig && siteKey) renderWidget(managedConfig, container);
	});
</script>

{#if active}
	<div class="mt-4" data-testid="captcha-widget" data-provider={provider}>
		<p class="mb-1 text-xs text-gray-500">Please complete the anti-bot check to reserve.</p>
		<div bind:this={container}></div>
		<input type="hidden" name="captcha_token" value={token} />
	</div>
{/if}
