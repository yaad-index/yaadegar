<script module lang="ts">
	// Provider wiring for the managed captcha widgets (ADR-0013). All three share the
	// same explicit-render shape — load an SDK, call <global>.render(el, {sitekey,
	// callback}) — differing only in the script URL and the global object name.
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
	}

	// Exported so tests can assert the URL/global mapping without a live network.
	export const captchaProviders: Record<string, ProviderConfig> = {
		turnstile: {
			src: 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit',
			global: 'turnstile'
		},
		hcaptcha: {
			src: 'https://js.hcaptcha.com/1/api.js?render=explicit',
			global: 'hcaptcha'
		},
		recaptcha: {
			src: 'https://www.google.com/recaptcha/api.js?render=explicit',
			global: 'grecaptcha'
		}
	};
</script>

<script lang="ts">
	import { browser } from '$app/environment';

	// provider selects the SDK; siteKey is the public render key. Both come from the
	// instance auth-methods config. The parent only mounts this when provider is a
	// known low-trust-enabled value, but we still guard on the lookup below.
	let { provider, siteKey }: { provider: string; siteKey: string } = $props();

	// The resolved challenge token, surfaced to the enclosing <form> through a hidden
	// input named captcha_token (the field the reserve action reads).
	let token = $state('');
	let container = $state<HTMLDivElement | null>(null);

	const config = $derived(captchaProviders[provider]);

	function apiFor(name: string): CaptchaApi | undefined {
		return (window as unknown as Record<string, CaptchaApi | undefined>)[name];
	}

	// Load the provider SDK once (deduped by a stable id), then explicit-render into
	// our container with a callback that stores the token. reCAPTCHA needs grecaptcha
	// .ready(); the others are ready as soon as the global exists.
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
		const ready = () => {
			const api = apiFor(cfg.global);
			if (api?.ready) api.ready(doRender);
			else doRender();
		};

		const id = `captcha-sdk-${cfg.global}`;
		if (document.getElementById(id)) {
			ready();
			return;
		}
		const script = document.createElement('script');
		script.id = id;
		script.src = cfg.src;
		script.async = true;
		script.defer = true;
		script.onload = ready;
		document.head.appendChild(script);
	}

	$effect(() => {
		if (!browser || !config || !siteKey || !container) return;
		renderWidget(config, container);
	});
</script>

{#if config && siteKey}
	<div class="mt-4" data-testid="captcha-widget" data-provider={provider}>
		<p class="mb-1 text-xs text-gray-500">Please complete the anti-bot check to reserve.</p>
		<div bind:this={container}></div>
		<input type="hidden" name="captcha_token" value={token} />
	</div>
{/if}
