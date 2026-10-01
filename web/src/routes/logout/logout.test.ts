import { describe, it, expect, vi, beforeEach } from 'vitest';

// Bug #238: logout must clear the session cookie with `secure` derived from the
// request protocol, not SvelteKit's non-localhost default. Over http the default
// would emit a Secure clear the browser drops, leaving the user signed in. This
// guards the caller wiring; session.test.ts guards the emitted attribute.

const clearSession = vi.fn();
vi.mock('$lib/server/session', () => ({
	clearSession: (...args: unknown[]) => clearSession(...args)
}));
// The unit project has no $lib alias; the real return-path guard is what is tested.
vi.mock('$lib/server/returnTo', () => import('../../lib/server/returnTo'));

import { POST } from './+server';

type Handler = (e: { cookies: Record<string, unknown>; url: URL }) => unknown;

// POST throws a redirect(303) after clearing; run it and ignore the thrown redirect.
async function run(url: string) {
	try {
		await (POST as unknown as Handler)({ cookies: {}, url: new URL(url) });
	} catch {
		/* redirect(303, '/login') throws by design */
	}
}

describe('logout POST clears with protocol-derived secure (#238)', () => {
	beforeEach(() => clearSession.mockReset());

	it('over http clears without Secure', async () => {
		await run('http://demo.wishes.example/logout');
		expect(clearSession).toHaveBeenCalledWith({}, false);
	});

	it('over https clears with Secure', async () => {
		await run('https://gifts.example/logout');
		expect(clearSession).toHaveBeenCalledWith({}, true);
	});
});

describe('logout passes a local return path on to login', () => {
	async function location(url: string) {
		try {
			await (POST as unknown as Handler)({ cookies: {}, url: new URL(url) });
		} catch (e) {
			return (e as { location: string }).location;
		}
		return '';
	}

	it('keeps a local path, encoded', async () => {
		expect(await location('https://t.example/logout?return_to=/settings')).toBe(
			'/login?return_to=%2Fsettings'
		);
	});

	it.each(['https://evil.example', '//evil.example', '/\\evil.example', ''])(
		'drops %j',
		async (raw) => {
			expect(await location(`https://t.example/logout?return_to=${encodeURIComponent(raw)}`)).toBe(
				'/login'
			);
		}
	);
});
