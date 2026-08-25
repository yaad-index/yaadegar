import { describe, it, expect } from 'vitest';

// Load the sources to walk and the doc to check as text, through Vite rather than the
// filesystem: `?raw` and import.meta.glob keep this dependency-free (no @types/node) and
// resolve the same way the app's own imports do. The glob eagerly imports every module's
// TEXT (not its exports — nothing here is executed), keyed by path.
import envExample from '../../../.env.example?raw';
const sourceFiles = import.meta.glob('/src/**/*.{ts,svelte}', {
	query: '?raw',
	import: 'default',
	eager: true
}) as Record<string, string>;

// web/.env.example is hand-maintained documentation of a machine-readable surface — the
// environment variables the web service actually consumes — and hand-maintained docs of a
// live surface drift silently. This is the web-side analogue of the backend's
// cmd/yaadegar/config_sample_test.go (ADR-0015 §4/§5): it derives the read set from the
// source itself and fails, in BOTH directions, when .env.example names a different set —
// an undocumented key the code reads, OR a documented key nothing reads. The
// both-directions half is what makes it a guard rather than a checklist, because drift
// here usually shows up as a stale leftover after a key is renamed or removed, not as a
// missing entry.
//
// WHAT THE WALK SEES, AND WHAT IT DOES NOT. The web service reads env through SvelteKit's
// `$env/dynamic/private`, but the reads are not all at the import site: +page.server.ts
// hands the whole env record to resolveRootPage(), which reads its keys off a plain
// parameter (rootPage.ts), one indirection removed from any `env` binding. So this guard
// does not track a binding — it matches env-var-shaped TOKENS (SCREAMING_SNAKE: an
// upper-case run with at least one underscore) appearing as property accesses anywhere in
// production source, exactly the way the Go guard matches names by regex. That shape
// captures BACKEND_ORIGIN and every YAADEGAR_* key wherever it is read, and excludes the
// underscore-free HTTP-verb accesses (.GET/.POST/.PATCH/...) that pepper the route
// modules. A key read as a lower-case or single-word property (e.g. a hypothetical
// `env.DEBUG`) would be missed; today's convention makes that surface empty, and a new
// single-word operator var would need adding to the shape below.
//
// TWO documented surfaces the walk structurally cannot discover, encoded as explicit
// exemptions rather than papered over:
//   - ADAPTER_VARS: ORIGIN/PORT/HOST are consumed by adapter-node's own server at runtime,
//     not by anything under src/, so no source walk can see them — yet they are real
//     operator configuration that belongs in .env.example. They are allowed to be
//     documented without a matching read. (The guard cannot enforce their PRESENCE — with
//     no src anchor there is nothing to compare against — so deleting one from the docs is
//     a blind spot; the guard governs code↔docs drift, and these have no code side.)
//   - NON_OPERATOR_READS: YAADEGAR_VERSION is read by version.ts but is the web image's own
//     build stamp (docs/docker-compose.yml header), deliberately NOT an operator knob, so
//     it is intentionally absent from .env.example. It is allowed to be read without being
//     documented.
// Both sets are small and stable; drift in the genuine operator surface still trips the
// guard.

// The two sides need different shapes, because their contexts have different ambiguity.
//
// READ side — a property access `.KEY`/['KEY'] in source. Bare upper-case is ambiguous
// here: the route modules are full of underscore-free HTTP-verb accesses (.GET/.POST/...)
// that are not env vars. Requiring at least one underscore (SCREAMING_SNAKE) cleanly
// separates BACKEND_ORIGIN / YAADEGAR_* from those verbs. The cost is a blind spot: a
// single-word env read (a hypothetical `env.DEBUG`) is not seen — acceptable because the
// web service's env convention is BACKEND_ORIGIN and the YAADEGAR_* family, and a new
// single-word operator var would surface on the DOC side and force a classification below.
const READ_TOKEN = '[A-Z][A-Z0-9]*(?:_[A-Z0-9]+)+';
const READ_RE = new RegExp(`[.[]\\s*['"]?(${READ_TOKEN})\\b`, 'g');

// DOC side — an assignment line in .env.example, active (`KEY=`) or a commented-out
// example (`#KEY=`); the custom-mode slots ship commented, and a commented example is
// still documentation. Here `KEY=` is unambiguous, so single-word keys count too — this
// is what lets ORIGIN/PORT/HOST be seen (and thus be genuinely governed by ADAPTER_VARS,
// rather than silently dropped by an underscore rule that would make that set dead code).
const DOC_RE = /^#?\s*([A-Z][A-Z0-9_]*)=/gm;

// Consumed by adapter-node at runtime, invisible to a src/ walk — documented without a read.
const ADAPTER_VARS = new Set(['ORIGIN', 'PORT', 'HOST']);
// Read by code but deliberately not an operator knob (the web build stamp) — read without a doc.
const NON_OPERATOR_READS = new Set(['YAADEGAR_VERSION']);

// The glob captures this test and the component tests too; a source walk of the app must
// exclude tests, spec/stub fixtures, and this guard itself (whose own regex literals name
// env vars). Paths are Vite-root-absolute, e.g. /src/lib/server/api.ts.
function isProductionSource(path: string): boolean {
	if (/\.(test|spec)\.ts$/.test(path)) return false; // unit + component tests
	if (/\.svelte\.(test|spec)\.ts$/.test(path)) return false; // svelte component tests
	if (path.includes('/lib/testing/')) return false; // stubs/fixtures, not production
	return true;
}

function collectReads(): Set<string> {
	const reads = new Set<string>();
	for (const [path, text] of Object.entries(sourceFiles)) {
		if (!isProductionSource(path)) continue;
		for (const m of text.matchAll(READ_RE)) reads.add(m[1]);
	}
	return reads;
}

function collectDocumented(): Set<string> {
	const documented = new Set<string>();
	for (const m of envExample.matchAll(DOC_RE)) documented.add(m[1]);
	return documented;
}

describe('web/.env.example', () => {
	const reads = collectReads();
	const documented = collectDocumented();

	it('finds the reads it is meant to find (the walk is not silently empty)', () => {
		// A positive control: if the token walk broke and matched nothing, every other
		// assertion would pass vacuously. Anchor on a key that is unambiguously read.
		expect(reads.has('BACKEND_ORIGIN')).toBe(true);
		expect([...reads].some((k) => k.startsWith('YAADEGAR_ROOT_'))).toBe(true);
	});

	it('documents every operator env var the web code reads', () => {
		const missing = [...reads]
			.filter((k) => !NON_OPERATOR_READS.has(k) && !documented.has(k))
			.sort();
		expect(
			missing,
			'web/.env.example is missing env vars the web code reads — document each ' +
				'(or, if it is genuinely not an operator knob, add it to NON_OPERATOR_READS with a reason)'
		).toEqual([]);
	});

	it('reads every operator env var it documents (no stale leftovers)', () => {
		const stale = [...documented].filter((k) => !ADAPTER_VARS.has(k) && !reads.has(k)).sort();
		expect(
			stale,
			'web/.env.example documents env vars nothing reads — remove them ' +
				'(or, if consumed by the adapter runtime rather than src/, add to ADAPTER_VARS with a reason)'
		).toEqual([]);
	});
});
