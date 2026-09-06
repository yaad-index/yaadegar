// The legal pages, /privacy and /terms (#300).
//
// Both routes ALWAYS exist and always render. What they render is the installation's own
// text when it supplies one, and an explicit PLACE HOLDER when it does not. The project
// ships no default legal wording, deliberately: real-looking policy text would put words
// in an operator's mouth for a jurisdiction the project knows nothing about, while a
// visible PLACE HOLDER cannot be mistaken for a policy. What the motivating case needs —
// Google refuses to publish an OAuth consent screen whose privacy-policy and
// terms-of-service URLs 404 — is a URL that resolves, not words the project wrote.
//
// The text lives on the WEB service, next to the YAADEGAR_ROOT_* slots rather than in the
// Go backend's config.example.yaml, for the same reason as those: the two services are
// separate images, and a key there would trip the backend's sample-config drift guard,
// which asserts in both directions (ADR-0015 §4/§5).
//
// Operator text is rendered as TEXT, never as HTML — same rule as the root-page slots — so
// there is no markup-injection surface. Unlike the root page there is no URL-valued slot
// here, so no scheme allowlist is needed: every value is inert content.
//
// WHY THIS IS NOT UNDER lib/server, where rootPage.ts sits. It is pure either way — a
// function of an env record it is handed, never of a live environment — but the page
// component needs the placeholder marker and the LegalPage type, and SvelteKit forbids a
// client module importing $lib/server. So it lives here and the one server-only step, the
// `$env/dynamic/private` read, stays in the route loaders.

export type LegalDocument = 'privacy' | 'terms';

export interface LegalPage {
	/** The document this page is. */
	doc: LegalDocument;
	/** Fixed page name. Not operator text, and not legal wording — just what the page is. */
	title: string;
	/** The env key an operator sets to replace the placeholder; shown on the placeholder. */
	envKey: string;
	/** The installation's text, split into paragraphs. Empty when nothing is configured. */
	paragraphs: string[];
	/** False when the installation supplied no text, i.e. this page is the placeholder. */
	configured: boolean;
}

/** The marker a visitor sees when the installation has published nothing. */
export const PLACEHOLDER_MARKER = 'PLACE HOLDER';

const TITLES: Record<LegalDocument, string> = {
	privacy: 'Privacy Policy',
	terms: 'Terms of Service'
};

const ENV_KEYS: Record<LegalDocument, string> = {
	privacy: 'YAADEGAR_PRIVACY_TEXT',
	terms: 'YAADEGAR_TERMS_TEXT'
};

/**
 * Split operator text into paragraphs on blank lines. Single newlines are left inside the
 * paragraph and preserved by the page's `whitespace-pre-line`, so an operator who pastes a
 * policy keeps the line structure they wrote either way. A value that is entirely
 * whitespace yields no paragraphs, which is what makes a blank env value read as "unset"
 * rather than as an empty policy.
 */
export function toParagraphs(raw: string | undefined): string[] {
	return (raw ?? '')
		.split(/\r?\n[ \t]*\r?\n/)
		.map((p) => p.trim())
		.filter((p) => p !== '');
}

/**
 * Resolve one legal page from the web service's environment. The two keys are read BY NAME
 * rather than through a lookup table so the .env.example drift guard — which matches env
 * names as property accesses in source — can see them.
 */
export function resolveLegalPage(
	doc: LegalDocument,
	e: {
		YAADEGAR_PRIVACY_TEXT?: string;
		YAADEGAR_TERMS_TEXT?: string;
		// The named keys document what is read; the index signature lets the whole
		// $env/dynamic/private record be passed straight in.
		[key: string]: string | undefined;
	}
): LegalPage {
	const raw = doc === 'privacy' ? e.YAADEGAR_PRIVACY_TEXT : e.YAADEGAR_TERMS_TEXT;
	const paragraphs = toParagraphs(raw);
	return {
		doc,
		title: TITLES[doc],
		envKey: ENV_KEYS[doc],
		paragraphs,
		configured: paragraphs.length > 0
	};
}
