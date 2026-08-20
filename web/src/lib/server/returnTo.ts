import { defineCookie } from './cookies';

// safeReturnTo validates a post-auth redirect target (#170) so it can only ever be a
// local path on this app. A crafted ?return_to=https://evil or //evil would otherwise
// let login/register bounce the user to another origin after they authenticate
// (open-redirect). Anything that is not a plain root-relative "/..." path — absolute
// URLs, protocol-relative "//host", or the "/\host" backslash trick browsers
// normalise to "//host" — is rejected as null and the caller falls back to its default.
export function safeReturnTo(raw: string | null | undefined): string | null {
	if (!raw) return null;
	if (!raw.startsWith('/')) return null;
	if (raw.startsWith('//') || raw.startsWith('/\\')) return null;
	return raw;
}

// The return path is stashed at register and consumed once at verify. httpOnly + lax,
// a one-hour life mirroring the verification link. Its set and clear come from one
// owner so they cannot drift — the clear used to omit `secure`, so over plain http a
// stale return path outlived its single use (#243).
export const RETURN_TO_COOKIE = 'return_to';

export const returnToCookie = defineCookie(RETURN_TO_COOKIE, {
	path: '/',
	httpOnly: true,
	sameSite: 'lax'
});
