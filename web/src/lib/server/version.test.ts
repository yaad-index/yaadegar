import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// version.ts reads the SvelteKit $env virtual module, absent under bare vitest. Stamp
// WEB_VERSION to a real value so the skew glue can be exercised (a "dev" default would
// suppress every case). BACKEND_ORIGIN targets a mock host for fetchBackendVersion.
vi.mock('$env/dynamic/private', () => ({
	env: { YAADEGAR_VERSION: '9.9.9-web', BACKEND_ORIGIN: 'http://backend.test' }
}));

import { versionSkewMessage, fetchBackendVersion, reportVersionSkew, WEB_VERSION } from './version';

function okJson(body: unknown): Response {
	return new Response(JSON.stringify(body), {
		status: 200,
		headers: { 'content-type': 'application/json' }
	});
}

afterEach(() => vi.restoreAllMocks());

describe('versionSkewMessage', () => {
	it('shouts only when both sides are stamped and differ', () => {
		expect(versionSkewMessage('0.13.0', '0.14.0')).toContain('web=0.13.0 api=0.14.0');
	});
	it('stays quiet when the two stamped versions match', () => {
		expect(versionSkewMessage('0.13.0', '0.13.0')).toBeNull();
	});
	it('stays quiet when either side is a dev/source build', () => {
		// The compose case: API git-describe vs web "dev" is not a mismatched release.
		expect(versionSkewMessage('dev', '0.13.0-5-gabc1234')).toBeNull();
		expect(versionSkewMessage('0.13.0', 'dev')).toBeNull();
	});
	it('stays quiet when either side is unknown or empty (unreachable/unstamped)', () => {
		expect(versionSkewMessage('0.13.0', 'unknown')).toBeNull();
		expect(versionSkewMessage('unknown', '0.13.0')).toBeNull();
		expect(versionSkewMessage('0.13.0', '')).toBeNull();
		expect(versionSkewMessage('', '0.13.0')).toBeNull();
	});
});

describe('fetchBackendVersion', () => {
	it('returns the backend version on a 200', async () => {
		vi.spyOn(globalThis, 'fetch').mockResolvedValue(okJson({ version: '1.2.3' }));
		expect(await fetchBackendVersion()).toBe('1.2.3');
	});
	it('reports unknown on a non-2xx', async () => {
		vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('', { status: 503 }));
		expect(await fetchBackendVersion()).toBe('unknown');
	});
	it('reports unknown when the backend is unreachable (fetch rejects)', async () => {
		vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('ECONNREFUSED'));
		expect(await fetchBackendVersion()).toBe('unknown');
	});
	it('reports unknown on a missing or non-string version field', async () => {
		vi.spyOn(globalThis, 'fetch').mockResolvedValue(okJson({ version: '' }));
		expect(await fetchBackendVersion()).toBe('unknown');
		vi.spyOn(globalThis, 'fetch').mockResolvedValue(okJson({ nope: true }));
		expect(await fetchBackendVersion()).toBe('unknown');
	});
	it('bounds the request with an abort timeout signal', async () => {
		// A slow backend that accepts and never answers is the case the catch cannot
		// cover; the timeout is what turns it into "unknown". Assert the signal is wired
		// so dropping it fails here rather than hanging /version in production.
		const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(okJson({ version: '1.0.0' }));
		await fetchBackendVersion();
		expect(fetchMock.mock.calls[0][1]?.signal).toBeInstanceOf(AbortSignal);
	});
	it('reports unknown when the request times out (abort fires)', async () => {
		vi.spyOn(globalThis, 'fetch').mockRejectedValue(
			new DOMException('The operation timed out.', 'TimeoutError')
		);
		expect(await fetchBackendVersion()).toBe('unknown');
	});
});

describe('reportVersionSkew', () => {
	let err: ReturnType<typeof vi.spyOn>;
	beforeEach(() => {
		err = vi.spyOn(console, 'error').mockImplementation(() => {});
	});
	it('logs once, naming both versions, on a stamped mismatch', async () => {
		vi.spyOn(globalThis, 'fetch').mockResolvedValue(okJson({ version: '1.0.0' }));
		await reportVersionSkew();
		expect(err).toHaveBeenCalledTimes(1);
		expect(err.mock.calls[0][0]).toContain(`web=${WEB_VERSION} api=1.0.0`);
	});
	it('is silent when the backend matches', async () => {
		vi.spyOn(globalThis, 'fetch').mockResolvedValue(okJson({ version: WEB_VERSION }));
		await reportVersionSkew();
		expect(err).not.toHaveBeenCalled();
	});
	it('is silent, and does not throw, when the backend is unreachable', async () => {
		vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('ECONNREFUSED'));
		await expect(reportVersionSkew()).resolves.toBeUndefined();
		expect(err).not.toHaveBeenCalled();
	});
});
