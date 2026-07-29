import { describe, it, expect } from 'vitest';
import { renderNote } from './markdown';

describe('renderNote', () => {
	it('renders light markdown to the allowed tags', () => {
		const html = renderNote('**bold** and *italic*\n\n- one\n- two');
		expect(html).toContain('<strong>bold</strong>');
		expect(html).toContain('<em>italic</em>');
		expect(html).toContain('<li>one</li>');
	});

	it('returns empty for null/empty notes', () => {
		expect(renderNote(null)).toBe('');
		expect(renderNote(undefined)).toBe('');
		expect(renderNote('')).toBe('');
	});

	// The load-bearing security test: a hostile note must be neutralized — no script,
	// no event handlers, no javascript:/data: hrefs, no arbitrary tags survive.
	it('neutralizes a hostile note', () => {
		const hostile = [
			'<script>alert(1)</script>',
			'<img src=x onerror=alert(1)>',
			'[click](javascript:alert(1))',
			'[data](data:text/html;base64,PHNjcmlwdD4=)',
			'<a href="http://ok.example" onclick="alert(1)">ok</a>',
			'<div onmouseover="alert(1)">hi</div>',
			'<iframe src="http://evil.example"></iframe>',
			'<style>body{display:none}</style>'
		].join('\n\n');
		const html = renderNote(hostile);

		expect(html).not.toContain('<script');
		expect(html).not.toContain('onerror');
		expect(html).not.toContain('onclick');
		expect(html).not.toContain('onmouseover');
		expect(html).not.toContain('<iframe');
		expect(html).not.toContain('<style');
		expect(html).not.toContain('<img');
		expect(html.toLowerCase()).not.toContain('javascript:');
		expect(html.toLowerCase()).not.toContain('data:text/html');
	});

	it('keeps safe http/https links and hardens them with rel/target', () => {
		const html = renderNote('see [the shop](https://shop.example/item)');
		expect(html).toContain('href="https://shop.example/item"');
		expect(html).toContain('rel="noopener noreferrer"');
		expect(html).toContain('target="_blank"');
	});
});
