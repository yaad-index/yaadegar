import { describe, it, expect } from 'vitest';
import { formatDay, tokenState } from './accessTokens';

describe('access token display helpers (ADR-0016 §7)', () => {
	it('formats a day in UTC, the same on the server and in the browser', () => {
		expect(formatDay('2027-06-15T23:30:00Z')).toBe('15 Jun 2027');
		expect(formatDay('2027-06-16T00:30:00+02:00')).toBe('15 Jun 2027');
	});

	it('says why a token does not work, revocation first', () => {
		expect(tokenState({ active: true, revoked_at: null })).toBe('active');
		expect(tokenState({ active: false, revoked_at: '2027-01-01T00:00:00Z' })).toBe('revoked');
		expect(tokenState({ active: false, revoked_at: null })).toBe('expired');
	});
});
