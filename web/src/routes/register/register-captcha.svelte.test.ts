import { describe, it, expect, vi } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';
import { ALTCHA_PROVIDER } from '$lib/components/CaptchaWidget.svelte';
import type { PageData } from './$types';

// #384: the register endpoint verifies captcha_token, so the page must render the
// widget that produces one and gate its submit until it resolves. Without the widget,
// every instance with a provider configured refuses every registration.
//
// The Altcha widget is a real browser web component with a solver worker that jsdom
// can neither register nor run, so the dynamic import is stubbed the same way the
// reserve page's widget test stubs it. The solve -> token -> register chain is a
// browser check, not a jsdom one.
vi.mock('altcha', () => ({}));

function pageData(over: Partial<PageData>): PageData {
	return {
		returnTo: '',
		registrationEnabled: true,
		captchaProvider: '',
		captchaSiteKey: '',
		...over
	} as unknown as PageData;
}

const submitButton = () => screen.getByRole('button', { name: 'Create account' });

describe('register page captcha widget (#384)', () => {
	it('renders the widget when the instance configures a provider', () => {
		render(Page, {
			data: pageData({ captchaProvider: 'turnstile', captchaSiteKey: 'site-key' }),
			form: null
		});
		expect(screen.getByTestId('captcha-widget')).toBeInTheDocument();
	});

	it('renders the widget for altcha, which carries no site key', () => {
		render(Page, {
			data: pageData({ captchaProvider: ALTCHA_PROVIDER, captchaSiteKey: '' }),
			form: null
		});
		expect(screen.getByTestId('captcha-widget')).toBeInTheDocument();
	});

	it('renders no widget and leaves submit live when captcha is disabled', () => {
		// The disabled-captcha instance must be byte-for-byte the page it was before
		// #384: no widget, nothing gating the button.
		render(Page, { data: pageData({ captchaProvider: '' }), form: null });
		expect(screen.queryByTestId('captcha-widget')).not.toBeInTheDocument();
		expect(submitButton()).toBeEnabled();
	});

	it('puts the captcha_token field inside the form, so a submit carries it', () => {
		// The widget's hidden input is only useful if it is inside the <form> the action
		// reads — rendering it anywhere on the page would pass a "widget present" check
		// and still post an empty token.
		const { container } = render(Page, {
			data: pageData({ captchaProvider: 'turnstile', captchaSiteKey: 'site-key' }),
			form: null
		});
		const field = container.querySelector('form input[name="captcha_token"]');
		expect(field).toBeInTheDocument();
	});

	it('disables submit until the check resolves, and says why', () => {
		// Not cosmetic: while unverified, altcha's injected `required` checkbox makes the
		// form invalid, so native constraint validation swallows the submit with no event
		// (#246). A live-looking button would be a second silent no-op.
		render(Page, {
			data: pageData({ captchaProvider: ALTCHA_PROVIDER, captchaSiteKey: '' }),
			form: null
		});
		expect(submitButton()).toBeDisabled();
		expect(screen.getByText('Complete the anti-bot check above first.')).toBeInTheDocument();
	});

	it('renders no widget when registration is disabled on the instance', () => {
		// #253 replaces the form with a not-enabled notice; there is no form to gate, so
		// the widget must not render (and must not fetch a challenge) on that branch.
		render(Page, {
			data: pageData({ registrationEnabled: false, captchaProvider: 'turnstile' }),
			form: null
		});
		expect(screen.queryByTestId('captcha-widget')).not.toBeInTheDocument();
	});
});
