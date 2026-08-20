import { describe, it, expect, vi } from 'vitest';
import { clearSession, SESSION_COOKIE } from './session';

// Bug #238: the clearing cookie must carry the same `secure` the cookie was set
// with, derived from the request protocol. Over plain http the clear must NOT be
// Secure — a browser refuses a Secure cookie over http, so a Secure clear is
// silently dropped and the (non-Secure) session cookie survives. This asserts the
// exact attribute the browser judges, on the unit that emits it.
describe('clearSession secure attribute (#238)', () => {
	it('over http clears WITHOUT Secure, so the browser accepts the clear', () => {
		const del = vi.fn();
		clearSession({ delete: del } as never, false);
		expect(del).toHaveBeenCalledWith(SESSION_COOKIE, { path: '/', secure: false });
	});

	it('over https clears WITH Secure', () => {
		const del = vi.fn();
		clearSession({ delete: del } as never, true);
		expect(del).toHaveBeenCalledWith(SESSION_COOKIE, { path: '/', secure: true });
	});
});
