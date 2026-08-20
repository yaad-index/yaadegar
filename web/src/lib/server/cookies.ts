import type { Cookies } from '@sveltejs/kit';

// A cookie's set and its clear must agree on their attributes, or the browser will
// not match the clear to the set and the cookie survives. Three shipped bugs came
// from a set and a clear drifting apart — most often the clear omitting `secure`
// and inheriting a framework default the set never used (#238 session, #243
// return_to, and the session clear before #238).
//
// defineCookie makes that drift unexpressible: the shared attributes (path,
// httpOnly, sameSite) are written once and BOTH set and clear read them from the
// same object. `secure` is deliberately NOT shared — it depends on the request
// protocol, so it is passed per call and REQUIRED on both set and clear, so a
// caller cannot silently leave it off the clear (the exact #238/#243 hole).

/** The attributes a cookie's set and clear must share — defined once per cookie. */
interface CookieShape {
	path: string;
	httpOnly: boolean;
	sameSite: 'lax' | 'strict' | 'none';
}

export interface ManagedCookie {
	set(cookies: Cookies, value: string, opts: { maxAge: number; secure: boolean }): void;
	clear(cookies: Cookies, opts: { secure: boolean }): void;
	read(cookies: Cookies): string | undefined;
}

/** Create the single owner of a cookie's lifecycle: set, clear, and read. */
export function defineCookie(name: string, shape: CookieShape): ManagedCookie {
	return {
		set(cookies, value, { maxAge, secure }) {
			cookies.set(name, value, { ...shape, secure, maxAge });
		},
		clear(cookies, { secure }) {
			// Same name and shape as set(), so path/httpOnly/sameSite cannot diverge;
			// `secure` mirrors what set() was given for this request.
			cookies.delete(name, { ...shape, secure });
		},
		read(cookies) {
			return cookies.get(name);
		}
	};
}
