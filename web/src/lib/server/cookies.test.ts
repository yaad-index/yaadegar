import { describe, it, expect, vi } from 'vitest';
import { defineCookie } from './cookies';

// The property the three cookie bugs all violated (#238 session, #243 return_to,
// and the session clear before #238): a cookie's set and its clear must emit the
// SAME attributes, or the browser won't match the clear to the set and the cookie
// survives. defineCookie exists to make that impossible to get wrong — so the test
// asserts set and clear AGREE, not that either is individually correct.
describe('defineCookie: set and clear agree', () => {
	const shape = { path: '/', httpOnly: true, sameSite: 'lax' as const };

	const spyCookies = () => ({ set: vi.fn(), delete: vi.fn(), get: vi.fn() });

	it.each([false, true])(
		'emits the same name + attributes on set and clear at secure=%s',
		(secure) => {
			const cookie = defineCookie('demo', shape);
			const cookies = spyCookies();

			cookie.set(cookies as never, 'value', { maxAge: 100, secure });
			cookie.clear(cookies as never, { secure });

			const [setName, , setOpts] = cookies.set.mock.calls[0];
			const [clearName, clearOpts] = cookies.delete.mock.calls[0];

			expect(clearName).toBe(setName);
			// The exact axes a set/clear mismatch drifts on — path/httpOnly/sameSite come
			// from one shared object, and secure is passed identically to both.
			for (const k of ['path', 'httpOnly', 'sameSite', 'secure'] as const) {
				expect(clearOpts[k]).toBe(setOpts[k]);
			}
			// And secure is a concrete boolean, never left undefined to inherit a default.
			expect(setOpts.secure).toBe(secure);
			expect(clearOpts.secure).toBe(secure);
		}
	);

	it('read returns the raw cookie value', () => {
		const cookie = defineCookie('demo', shape);
		const cookies = { ...spyCookies(), get: vi.fn().mockReturnValue('stored') };
		expect(cookie.read(cookies as never)).toBe('stored');
		expect(cookies.get).toHaveBeenCalledWith('demo');
	});
});
