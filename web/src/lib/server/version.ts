import { env } from '$env/dynamic/private';

// The web build version, baked into the image as YAADEGAR_VERSION by the publish
// workflow (#268) and read here at runtime. A source or local build (npm run dev,
// docker compose from source) leaves it unset, which reads as "dev" — the same
// default the Dockerfile stamps, and treated as unstamped by the skew check below.
export const WEB_VERSION = env.YAADEGAR_VERSION || 'dev';

// Where the backend lives (server-side only), same default as lib/server/api.ts.
const BACKEND_ORIGIN = env.BACKEND_ORIGIN ?? 'http://localhost:8080';

// A version poll must be bounded: a backend that accepts the connection and then
// never answers would leave the fetch awaiting forever — and this is the endpoint
// whose whole job is to be polled. 5s mirrors ADR-0013's bounded verify timeout.
const BACKEND_VERSION_TIMEOUT_MS = 5000;

// A version is "stamped" when it is a real published build, not a dev/source build
// or an unreachable backend. Only a mismatch between two stamped versions is a
// mismatched *release* pair worth reporting: a source build legitimately runs
// web="dev" against an API reporting its VCS commit, and that is not skew.
function isStamped(version: string): boolean {
	return version !== '' && version !== 'dev' && version !== 'unknown';
}

// versionSkewMessage returns the error line to log when the frontend and backend are
// a mismatched published pair, or null when there is nothing to report — either
// they match, or at least one side is unstamped (a dev/source build or an
// unreachable backend), where a mismatch is expected rather than a fault. Pure over
// its inputs so every branch is unit-testable without a network or an env.
export function versionSkewMessage(web: string, api: string): string | null {
	if (isStamped(web) && isStamped(api) && web !== api) {
		return `frontend/backend version mismatch: web=${web} api=${api} — the images are not a matched pair (ADR-0014); server-rendered pages may break`;
	}
	return null;
}

// fetchBackendVersion reads the API's build version from its unauthenticated,
// instance-level GET /api/v1/version (ADR-0014 §3). No X-Forwarded-Host: the endpoint
// skips tenant resolution, so no tenant is needed. Never throws or hangs — a slow
// (bounded by the timeout), erroring, or unreachable backend reports "unknown", so
// startup and the /version poll degrade rather than fail or wait forever.
export async function fetchBackendVersion(): Promise<string> {
	try {
		const res = await fetch(BACKEND_ORIGIN + '/api/v1/version', {
			signal: AbortSignal.timeout(BACKEND_VERSION_TIMEOUT_MS)
		});
		if (!res.ok) return 'unknown';
		const body = (await res.json()) as { version?: unknown };
		return typeof body.version === 'string' && body.version !== '' ? body.version : 'unknown';
	} catch {
		return 'unknown';
	}
}

// reportVersionSkew reads the backend version once and logs loudly on a mismatched
// pair. Called once at server startup (hooks.server.ts). It logs and never throws or
// blocks: refusing to start would turn a staged rollout into an outage (ADR-0014 §3),
// so skew ends the silence with an error line, nothing more.
export async function reportVersionSkew(): Promise<void> {
	const message = versionSkewMessage(WEB_VERSION, await fetchBackendVersion());
	if (message) console.error(`[version] ${message}`);
}
