import { Marked } from 'marked';
import sanitizeHtml from 'sanitize-html';

// renderNote turns an item's light-markdown note into HTML that is safe to inject
// with {@html}. It runs SERVER-SIDE only (called from load): the browser must
// never receive a raw note for client-side rendering. Two layers:
//   1. marked parses markdown with raw-HTML emission dropped (defense in depth), then
//   2. sanitize-html enforces a tight allowlist — the real, enforced trust boundary
//      regardless of what the parser emits.
// The caller stores the result in its own PageData field; {@html} only ever touches
// this sanitized output (ADR-0006 security boundary).

// A markdown parser that drops raw HTML blocks outright — belt to sanitize-html's
// braces.
const md = new Marked({ gfm: true });
md.use({ renderer: { html: () => '' } });

// Light-formatting allowlist only. Everything marked can emit that is not here —
// and any raw HTML that slipped through — is discarded.
const SANITIZE: sanitizeHtml.IOptions = {
	allowedTags: [
		'p',
		'br',
		'strong',
		'em',
		'ul',
		'ol',
		'li',
		'a',
		'code',
		'pre',
		'blockquote',
		'h3'
	],
	// href plus the rel/target that transformTags injects (they are stripped unless
	// allowed); the transform forces their values, so a note cannot set its own.
	allowedAttributes: { a: ['href', 'rel', 'target'] },
	// Block javascript:/data: etc. — only real navigation protocols.
	allowedSchemes: ['http', 'https', 'mailto'],
	disallowedTagsMode: 'discard',
	// Harden every rendered link.
	transformTags: {
		a: sanitizeHtml.simpleTransform('a', { rel: 'noopener noreferrer', target: '_blank' })
	}
};

export function renderNote(note: string | null | undefined): string {
	if (!note) return '';
	const rawHtml = md.parse(note, { async: false }) as string;
	return sanitizeHtml(rawHtml, SANITIZE);
}
