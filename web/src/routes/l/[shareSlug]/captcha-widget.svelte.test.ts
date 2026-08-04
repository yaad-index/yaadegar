import { describe, it, expect } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';
import CaptchaWidget, { captchaProviders } from '$lib/components/CaptchaWidget.svelte';
import type { PageData } from './$types';

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

	it('renders nothing for an unknown provider', () => {
		render(CaptchaWidget, { provider: 'nope', siteKey: 'sk' });
		expect(screen.queryByTestId('captcha-widget')).not.toBeInTheDocument();
	});

	it('renders nothing without a site key', () => {
		render(CaptchaWidget, { provider: 'turnstile', siteKey: '' });
		expect(screen.queryByTestId('captcha-widget')).not.toBeInTheDocument();
	});

	it('maps each managed provider to its SDK URL and global', () => {
		expect(captchaProviders.turnstile.global).toBe('turnstile');
		expect(captchaProviders.turnstile.src).toContain('challenges.cloudflare.com');
		expect(captchaProviders.hcaptcha.global).toBe('hcaptcha');
		expect(captchaProviders.hcaptcha.src).toContain('hcaptcha.com');
		expect(captchaProviders.recaptcha.global).toBe('grecaptcha');
		expect(captchaProviders.recaptcha.src).toContain('recaptcha');
	});
});
