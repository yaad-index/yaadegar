// Personal access tokens (ADR-0016), as the settings screen offers and shows them.

// formatDay renders an instant as a day, in UTC so the page renders the same on the
// server and in the browser. A token's dates are informational (created, expires,
// last used to within five minutes), so the day is the useful precision.
export function formatDay(iso: string): string {
	return new Intl.DateTimeFormat('en-GB', { dateStyle: 'medium', timeZone: 'UTC' }).format(
		new Date(iso)
	);
}

export type TokenState = 'active' | 'revoked' | 'expired';

// tokenState says why a token does or does not work: revoked wins over expired,
// because revoking is what the owner did.
export function tokenState(t: { active: boolean; revoked_at: string | null }): TokenState {
	if (t.active) return 'active';
	return t.revoked_at ? 'revoked' : 'expired';
}
