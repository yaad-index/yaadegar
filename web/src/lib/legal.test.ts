import { describe, it, expect } from 'vitest';
import { resolveLegalPage, toParagraphs, PLACEHOLDER_MARKER } from './legal';

describe('resolveLegalPage — placeholder vs configured (#300)', () => {
	it('is a placeholder when nothing is configured — the property the whole issue rests on', () => {
		for (const doc of ['privacy', 'terms'] as const) {
			expect(resolveLegalPage(doc, {})).toMatchObject({ configured: false, paragraphs: [] });
		}
	});

	it('treats a blank or whitespace-only value as unset, not as an empty policy', () => {
		expect(resolveLegalPage('privacy', { YAADEGAR_PRIVACY_TEXT: '' }).configured).toBe(false);
		expect(resolveLegalPage('privacy', { YAADEGAR_PRIVACY_TEXT: '   ' }).configured).toBe(false);
		expect(resolveLegalPage('terms', { YAADEGAR_TERMS_TEXT: '\n\n \t\n' }).configured).toBe(false);
	});

	it('shows the installation text when it is set', () => {
		const page = resolveLegalPage('privacy', { YAADEGAR_PRIVACY_TEXT: 'We keep only your list.' });
		expect(page).toMatchObject({ configured: true, paragraphs: ['We keep only your list.'] });
	});

	it('reads each document from its OWN key, and one document does not fill in the other', () => {
		const e = { YAADEGAR_PRIVACY_TEXT: 'privacy words' };
		expect(resolveLegalPage('privacy', e).paragraphs).toEqual(['privacy words']);
		// The mutation this catches: a resolver that reads a single key for both documents
		// would silently publish the privacy text as the terms of service.
		expect(resolveLegalPage('terms', e).configured).toBe(false);

		const t = { YAADEGAR_TERMS_TEXT: 'terms words' };
		expect(resolveLegalPage('terms', t).paragraphs).toEqual(['terms words']);
		expect(resolveLegalPage('privacy', t).configured).toBe(false);
	});

	it('carries the fixed title and the env key for each document', () => {
		expect(resolveLegalPage('privacy', {})).toMatchObject({
			doc: 'privacy',
			title: 'Privacy Policy',
			envKey: 'YAADEGAR_PRIVACY_TEXT'
		});
		expect(resolveLegalPage('terms', {})).toMatchObject({
			doc: 'terms',
			title: 'Terms of Service',
			envKey: 'YAADEGAR_TERMS_TEXT'
		});
	});

	it('ships no default legal wording — an unconfigured page carries no text to mistake for one', () => {
		// The point of the decision (issue #300): the placeholder is a marker, and the module
		// exports no shipped policy prose that a page could fall back to.
		expect(PLACEHOLDER_MARKER).toBe('PLACE HOLDER');
		expect(resolveLegalPage('privacy', {}).paragraphs).toEqual([]);
		expect(resolveLegalPage('terms', {}).paragraphs).toEqual([]);
	});
});

describe('toParagraphs', () => {
	it('splits on a blank line', () => {
		expect(toParagraphs('one\n\ntwo')).toEqual(['one', 'two']);
	});

	it('keeps single newlines inside a paragraph (the page preserves them)', () => {
		expect(toParagraphs('line one\nline two')).toEqual(['line one\nline two']);
	});

	it('splits on a blank line that carries spaces or tabs, and on CRLF', () => {
		expect(toParagraphs('one\n \t \ntwo')).toEqual(['one', 'two']);
		expect(toParagraphs('one\r\n\r\ntwo')).toEqual(['one', 'two']);
	});

	it('drops empty runs so leading, trailing and repeated blank lines add no empty paragraphs', () => {
		expect(toParagraphs('\n\none\n\n\n\ntwo\n\n')).toEqual(['one', 'two']);
	});

	it('is empty for undefined and for whitespace', () => {
		expect(toParagraphs(undefined)).toEqual([]);
		expect(toParagraphs('   \n  ')).toEqual([]);
	});
});
