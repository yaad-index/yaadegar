import { describe, it, expect } from 'vitest';
import { resolveRootPage, sanitizeCtaHref, BUNDLED_STRINGS } from './rootPage';

describe('resolveRootPage — mode selection', () => {
	it('is bundled when nothing is configured (the property most likely to regress silently)', () => {
		expect(resolveRootPage({})).toEqual({ mode: 'bundled' });
		expect(resolveRootPage({ YAADEGAR_ROOT_PAGE: '' })).toEqual({ mode: 'bundled' });
		expect(resolveRootPage({ YAADEGAR_ROOT_PAGE: '   ' })).toEqual({ mode: 'bundled' });
	});

	it('is bundled for the explicit value and for an unrecognized one (fail-safe to today)', () => {
		expect(resolveRootPage({ YAADEGAR_ROOT_PAGE: 'bundled' })).toEqual({ mode: 'bundled' });
		expect(resolveRootPage({ YAADEGAR_ROOT_PAGE: 'marketing' })).toEqual({ mode: 'bundled' });
		expect(resolveRootPage({ YAADEGAR_ROOT_PAGE: 'CUSTM' })).toEqual({ mode: 'bundled' });
	});

	it('redirects to login when asked, ignoring any slot values', () => {
		expect(resolveRootPage({ YAADEGAR_ROOT_PAGE: 'login', YAADEGAR_ROOT_HEADLINE: 'x' })).toEqual({
			mode: 'login'
		});
	});

	it('is case- and whitespace-insensitive on the mode', () => {
		expect(resolveRootPage({ YAADEGAR_ROOT_PAGE: ' Login ' })).toEqual({ mode: 'login' });
		expect(resolveRootPage({ YAADEGAR_ROOT_PAGE: 'CUSTOM' }).mode).toBe('custom');
	});
});

describe('resolveRootPage — custom strings', () => {
	it('uses each operator-supplied slot', () => {
		const r = resolveRootPage({
			YAADEGAR_ROOT_PAGE: 'custom',
			YAADEGAR_ROOT_HEADLINE: 'Our family wishlist',
			YAADEGAR_ROOT_SUBHEAD: 'Reserve without spoiling the surprise.',
			YAADEGAR_ROOT_CTA_LABEL: 'Get started',
			YAADEGAR_ROOT_CTA_HREF: 'https://example.org/signup',
			YAADEGAR_ROOT_TRUST_LINE: 'Run by us, for us'
		});
		expect(r).toEqual({
			mode: 'custom',
			strings: {
				headline: 'Our family wishlist',
				subhead: 'Reserve without spoiling the surprise.',
				ctaLabel: 'Get started',
				ctaHref: 'https://example.org/signup',
				trustLine: 'Run by us, for us'
			}
		});
	});

	it('inherits the bundled wording for any unset or blank slot (custom-with-nothing == bundled)', () => {
		expect(resolveRootPage({ YAADEGAR_ROOT_PAGE: 'custom' })).toEqual({
			mode: 'custom',
			strings: BUNDLED_STRINGS
		});
		const partial = resolveRootPage({
			YAADEGAR_ROOT_PAGE: 'custom',
			YAADEGAR_ROOT_HEADLINE: 'Just the headline',
			YAADEGAR_ROOT_SUBHEAD: '   '
		});
		expect(partial.mode).toBe('custom');
		if (partial.mode === 'custom') {
			expect(partial.strings.headline).toBe('Just the headline');
			expect(partial.strings.subhead).toBe(BUNDLED_STRINGS.subhead);
			expect(partial.strings.ctaLabel).toBe(BUNDLED_STRINGS.ctaLabel);
		}
	});

	it('rejects a javascript: CTA href and falls back to the bundled destination', () => {
		const r = resolveRootPage({
			YAADEGAR_ROOT_PAGE: 'custom',
			YAADEGAR_ROOT_CTA_HREF: 'javascript:alert(document.cookie)'
		});
		expect(r.mode).toBe('custom');
		if (r.mode === 'custom') {
			expect(r.strings.ctaHref).toBe(BUNDLED_STRINGS.ctaHref);
		}
	});
});

describe('sanitizeCtaHref', () => {
	const fb = '/login';

	it('allows absolute http and https URLs', () => {
		expect(sanitizeCtaHref('http://example.com/x', fb)).toBe('http://example.com/x');
		expect(sanitizeCtaHref('https://example.com/x?q=1#f', fb)).toBe('https://example.com/x?q=1#f');
	});

	it('allows a site-relative path', () => {
		expect(sanitizeCtaHref('/signup', fb)).toBe('/signup');
		expect(sanitizeCtaHref('/lists?tab=all', fb)).toBe('/lists?tab=all');
	});

	it('rejects live-scheme values, returning the fallback', () => {
		expect(sanitizeCtaHref('javascript:alert(1)', fb)).toBe(fb);
		expect(sanitizeCtaHref('JavaScript:alert(1)', fb)).toBe(fb);
		expect(sanitizeCtaHref('data:text/html,<script>1</script>', fb)).toBe(fb);
		expect(sanitizeCtaHref('mailto:a@b.com', fb)).toBe(fb);
		expect(sanitizeCtaHref('ftp://host/f', fb)).toBe(fb);
	});

	it('rejects authority-smuggling relatives — //host and the backslash forms alike', () => {
		// A special-scheme URL parser treats "\" as "/" in the authority state, so all of
		// these resolve to host "evil.example" on the page. The old string-prefix check
		// caught only the first.
		expect(sanitizeCtaHref('//evil.example', fb)).toBe(fb);
		expect(sanitizeCtaHref('/\\evil.example', fb)).toBe(fb);
		expect(sanitizeCtaHref('/\\/evil.example', fb)).toBe(fb);
		expect(sanitizeCtaHref('\\\\evil.example', fb)).toBe(fb);
	});

	it('accepts an ordinary same-site relative that merely contains a later backslash or dots', () => {
		expect(sanitizeCtaHref('/a/b', fb)).toBe('/a/b');
		expect(sanitizeCtaHref('/a/../b', fb)).toBe('/a/../b');
	});

	it('falls back for an unset or blank value', () => {
		expect(sanitizeCtaHref(undefined, fb)).toBe(fb);
		expect(sanitizeCtaHref('   ', fb)).toBe(fb);
	});
});
