// Root-page configuration for a signed-out visitor (ADR-0015, #256).
//
// One operator env key, YAADEGAR_ROOT_PAGE, names what the site root is:
//   - bundled (default) — the shipped marketing landing, unchanged.
//   - login             — redirect signed-out visitors to /login (a private instance).
//   - custom            — the bundled layout with operator-supplied text (the slots below).
//
// The setting lives on the WEB service, not the Go backend: it is read here from the
// web image's own environment, and config.example.yaml is deliberately untouched — a
// key there would trip the backend's sample-config drift guard, which asserts in both
// directions (ADR-0015 §4/§5).
//
// custom is overridable STRINGS, never operator markup: each slot is rendered by the
// page as text or as an escaped attribute value, so there is no markup-injection
// surface. The one slot with a real surface is the CTA destination — a URL-valued
// attribute where a javascript:/data: scheme is a live click-time vector even under
// attribute escaping — so it is passed through a scheme allowlist here before it ever
// reaches the template (ADR-0015 §2/§3).
//
// Factored as a pure function of its env input so every branch is unit-testable
// without a real environment, mirroring version.ts.

export interface RootStrings {
	headline: string;
	subhead: string;
	ctaLabel: string;
	ctaHref: string;
	trustLine: string;
}

export type RootPage =
	{ mode: 'bundled' } | { mode: 'login' } | { mode: 'custom'; strings: RootStrings };

// The shipped copy, and the single source of truth for it: an unset custom slot falls
// back to exactly what the bundled landing shows, so `custom` with nothing set is the
// bundled wording. (The bundled hero renders its headline with an explicit line break
// in the template; as a plain-text default here it simply wraps naturally.)
export const BUNDLED_STRINGS: RootStrings = {
	headline: 'Wishes worth keeping — surprises kept safe',
	subhead:
		'Yaadegar is a friendly, self-hosted gift list web app. Share your wishlist with friends and family without ruining the magic — givers can coordinate and reserve secretly, while you stay blissfully in the dark.',
	ctaLabel: 'Create your list',
	ctaHref: '/login',
	trustLine: 'Free & Open Source'
};

/** A trimmed non-empty string, or undefined — so a blank env value inherits the default. */
function nonEmpty(v: string | undefined): string | undefined {
	const t = v?.trim();
	return t ? t : undefined;
}

/**
 * The CTA destination allowlist: site-relative paths and absolute http/https URLs only.
 * A `javascript:`/`data:`/`mailto:` scheme, a protocol-relative `//host`, or any
 * unparseable value falls back to the bundled default rather than reaching the page —
 * attribute escaping stops a value breaking out of the attribute, but not a live scheme
 * on click, so this is where that is closed (ADR-0015 §2/§3).
 */
const RELATIVE_BASE = 'http://internal.invalid/';

export function sanitizeCtaHref(raw: string | undefined, fallback: string): string {
	const v = nonEmpty(raw);
	if (!v) return fallback;
	// Absolute URL (has its own scheme): http/https pass; javascript:/data:/mailto:/ftp:
	// and every other scheme are rejected.
	let absolute: URL | undefined;
	try {
		absolute = new URL(v);
	} catch {
		// not absolute — a relative reference, handled below
	}
	if (absolute) {
		return absolute.protocol === 'http:' || absolute.protocol === 'https:' ? v : fallback;
	}
	// A relative reference: resolve it with the SAME URL machinery a browser uses, against
	// a placeholder base, and accept it only if no authority was smuggled in — i.e. it
	// stayed same-origin. A raw string-prefix check ("/ but not //") misses the backslash
	// forms: for a special scheme the parser treats "\" as "/" in the authority state, so
	// "/\evil" and "/\/evil" resolve to host "evil" exactly as "//evil" does.
	try {
		const u = new URL(v, RELATIVE_BASE);
		if (u.origin === new URL(RELATIVE_BASE).origin) return v;
	} catch {
		// unparseable even against a base — fall through
	}
	return fallback;
}

/**
 * Resolve the root-page behaviour from the web service's environment. Any value of
 * YAADEGAR_ROOT_PAGE other than `login` or `custom` — unset, `bundled`, or a typo —
 * resolves to `bundled`, so an instance that configures nothing (or configures
 * nonsense) behaves exactly as it does today.
 */
export function resolveRootPage(e: {
	YAADEGAR_ROOT_PAGE?: string;
	YAADEGAR_ROOT_HEADLINE?: string;
	YAADEGAR_ROOT_SUBHEAD?: string;
	YAADEGAR_ROOT_CTA_LABEL?: string;
	YAADEGAR_ROOT_CTA_HREF?: string;
	YAADEGAR_ROOT_TRUST_LINE?: string;
	// The named keys document what is read; the index signature lets the whole
	// $env/dynamic/private record be passed straight in.
	[key: string]: string | undefined;
}): RootPage {
	const mode = nonEmpty(e.YAADEGAR_ROOT_PAGE)?.toLowerCase();
	if (mode === 'login') return { mode: 'login' };
	if (mode === 'custom') {
		return {
			mode: 'custom',
			strings: {
				headline: nonEmpty(e.YAADEGAR_ROOT_HEADLINE) ?? BUNDLED_STRINGS.headline,
				subhead: nonEmpty(e.YAADEGAR_ROOT_SUBHEAD) ?? BUNDLED_STRINGS.subhead,
				ctaLabel: nonEmpty(e.YAADEGAR_ROOT_CTA_LABEL) ?? BUNDLED_STRINGS.ctaLabel,
				ctaHref: sanitizeCtaHref(e.YAADEGAR_ROOT_CTA_HREF, BUNDLED_STRINGS.ctaHref),
				trustLine: nonEmpty(e.YAADEGAR_ROOT_TRUST_LINE) ?? BUNDLED_STRINGS.trustLine
			}
		};
	}
	return { mode: 'bundled' };
}
