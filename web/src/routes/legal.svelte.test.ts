import { describe, it, expect, afterEach } from 'vitest';
import { env } from '$env/dynamic/private';
import { load as loadPrivacy } from './privacy/+page.server';
import { load as loadTerms } from './terms/+page.server';

// The loader wiring for /privacy and /terms (#300). resolveLegalPage's branches are unit-
// tested in ../lib/legal.test.ts and the rendering in ../lib/components/
// LegalDocument.svelte.test.ts; what is only true here is that each route reads the live
// environment and that neither route gates on a session.

const anonymous = { locals: {} } as never;

describe('legal-page loaders', () => {
	afterEach(() => {
		for (const k of Object.keys(env)) delete env[k];
	});

	it('serves both pages to a signed-out visitor — no redirect, no session needed', () => {
		// The requirement Google's consent-screen verification depends on: an unauthenticated
		// fetch must return the page rather than a redirect to /login.
		expect(loadPrivacy(anonymous)).toMatchObject({ page: { doc: 'privacy', configured: false } });
		expect(loadTerms(anonymous)).toMatchObject({ page: { doc: 'terms', configured: false } });
	});

	it('reads the live environment for each document', () => {
		env.YAADEGAR_PRIVACY_TEXT = 'Our privacy words.';
		env.YAADEGAR_TERMS_TEXT = 'Our terms words.';
		expect(loadPrivacy(anonymous)).toMatchObject({
			page: { configured: true, paragraphs: ['Our privacy words.'] }
		});
		expect(loadTerms(anonymous)).toMatchObject({
			page: { configured: true, paragraphs: ['Our terms words.'] }
		});
	});

	it('does not cross the two documents', () => {
		// Setting one leaves the other a placeholder; a loader wired to the wrong key would
		// publish the privacy text as the terms of service.
		env.YAADEGAR_PRIVACY_TEXT = 'Our privacy words.';
		expect(loadPrivacy(anonymous)).toMatchObject({ page: { configured: true } });
		expect(loadTerms(anonymous)).toMatchObject({ page: { configured: false } });
	});
});
