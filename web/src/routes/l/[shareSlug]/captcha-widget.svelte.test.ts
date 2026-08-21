import { describe, it, expect, vi } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';
import CaptchaWidget, {
	captchaProviders,
	ALTCHA_PROVIDER,
	ALTCHA_CHALLENGE_URL
} from '$lib/components/CaptchaWidget.svelte';
import type { PageData } from './$types';

// The Altcha widget is a real browser web component with a solver worker; jsdom can
// neither register nor run it (the same SDK-load class of behavior cut 1 learned only
// a real browser can verify). Stub the dynamic import so the component's gating/
// template still renders here; the solve→token→reserve chain is covered by the
// headless-chrome harness instead.
vi.mock('altcha', () => ({}));

// A minimal full_guest (low-trust) giver page, tier signalled by email_required /
// account_required both false. Captcha fields overridden per case.
function pageData(over: Partial<PageData>): PageData {
	return {
		closed: false,
		list: { title: 'Birthday', email_required: false, account_required: false, items: [] },
		reservedItemIds: [],
		pledged: {},
		noteHtml: {},
		descriptionHtml: '',
		accountRequired: false,
		loggedIn: false,
		registrationEnabled: false,
		captchaProvider: '',
		captchaSiteKey: '',
		reservePath: '',
		shareSlug: 's1',
		...over
	} as unknown as PageData;
}

describe('reserve page captcha widget gating (ADR-0013)', () => {
	it('renders the widget on a low-trust list when a provider is configured', () => {
		render(Page, {
			data: pageData({ captchaProvider: 'turnstile', captchaSiteKey: 'site-key' }),
			form: null
		});
		expect(screen.getByTestId('captcha-widget')).toBeInTheDocument();
	});

	it('renders no widget when captcha is disabled (no provider)', () => {
		render(Page, { data: pageData({ captchaProvider: '', captchaSiteKey: '' }), form: null });
		expect(screen.queryByTestId('captcha-widget')).not.toBeInTheDocument();
	});

	it('renders the widget for altcha on a low-trust list with no site key', () => {
		// Altcha is self-hosted proof-of-work: it needs no site key, so the widget must
		// render on the provider name alone (unlike the managed providers).
		render(Page, {
			data: pageData({ captchaProvider: ALTCHA_PROVIDER, captchaSiteKey: '' }),
			form: null
		});
		expect(screen.getByTestId('captcha-widget')).toBeInTheDocument();
	});

	it('renders no widget on an account-required (registered) list', () => {
		// A registered-tier list shows the account-required prompt, not the reserve form,
		// so the widget never mounts even if a provider is configured.
		render(Page, {
			data: pageData({
				accountRequired: true,
				captchaProvider: 'turnstile',
				captchaSiteKey: 'site-key'
			}),
			form: null
		});
		expect(screen.queryByTestId('captcha-widget')).not.toBeInTheDocument();
	});
});

describe('CaptchaWidget', () => {
	it('renders a hidden captcha_token field for a known provider + site key', () => {
		const { container } = render(CaptchaWidget, { provider: 'hcaptcha', siteKey: 'sk' });
		expect(screen.getByTestId('captcha-widget')).toHaveAttribute('data-provider', 'hcaptcha');
		const hidden = container.querySelector('input[name="captcha_token"]');
		expect(hidden).toBeInTheDocument();
		expect(hidden).toHaveAttribute('type', 'hidden');
	});

	it('explains the check in action-neutral terms, not just reserve (#251)', () => {
		// The widget gates both reserve and pledge (#250), so while unverified its notice
		// must not name only reserve — a visitor who opened "Chip in instead" reads it too.
		render(CaptchaWidget, { provider: 'hcaptcha', siteKey: 'sk' });
		const notice = screen.getByText(/complete the anti-bot check/i);
		expect(notice).toBeInTheDocument();
		expect(notice.textContent?.toLowerCase()).not.toContain('reserve');
	});

	it('renders nothing for an unknown provider', () => {
		render(CaptchaWidget, { provider: 'nope', siteKey: 'sk' });
		expect(screen.queryByTestId('captcha-widget')).not.toBeInTheDocument();
	});

	it('renders nothing for a managed provider without a site key', () => {
		render(CaptchaWidget, { provider: 'turnstile', siteKey: '' });
		expect(screen.queryByTestId('captcha-widget')).not.toBeInTheDocument();
	});

	it('renders the altcha widget with no site key and a hidden token field', () => {
		const { container } = render(CaptchaWidget, { provider: ALTCHA_PROVIDER, siteKey: '' });
		expect(screen.getByTestId('captcha-widget')).toHaveAttribute('data-provider', ALTCHA_PROVIDER);
		expect(container.querySelector('input[name="captcha_token"]')).toBeInTheDocument();
	});

	it('points altcha at the same-origin challenge endpoint', () => {
		// A same-origin path (proxied to the backend), never a third-party CDN — the
		// zero-third-party guarantee.
		expect(ALTCHA_CHALLENGE_URL).toBe('/api/v1/public/captcha/challenge');
		expect(ALTCHA_CHALLENGE_URL.startsWith('/')).toBe(true);
	});

	it('maps each managed provider to its SDK URL, global, and ready policy', () => {
		// Only reCAPTCHA gates render on ready(); Turnstile forbids ready() under
		// async/defer, so it (and hCaptcha) must render directly on onload.
		expect(captchaProviders.turnstile.global).toBe('turnstile');
		expect(captchaProviders.turnstile.src).toContain('challenges.cloudflare.com');
		expect(captchaProviders.turnstile.usesReady).toBe(false);
		expect(captchaProviders.hcaptcha.global).toBe('hcaptcha');
		expect(captchaProviders.hcaptcha.src).toContain('hcaptcha.com');
		expect(captchaProviders.hcaptcha.usesReady).toBe(false);
		expect(captchaProviders.recaptcha.global).toBe('grecaptcha');
		expect(captchaProviders.recaptcha.src).toContain('recaptcha');
		expect(captchaProviders.recaptcha.usesReady).toBe(true);
	});
});
