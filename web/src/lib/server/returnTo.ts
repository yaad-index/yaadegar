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
