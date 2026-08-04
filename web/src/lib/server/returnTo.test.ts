import { describe, it, expect } from 'vitest';
import { safeReturnTo } from './returnTo';

// #170 open-redirect guard: only a plain local path may become a post-auth redirect.
describe('safeReturnTo', () => {
	it('accepts a plain root-relative path', () => {
		expect(safeReturnTo('/reserve/abc')).toBe('/reserve/abc');
	});

	it('rejects an absolute URL', () => {
		expect(safeReturnTo('https://evil.example/phish')).toBeNull();
	});

	it('rejects a protocol-relative //host', () => {
		expect(safeReturnTo('//evil.example')).toBeNull();
	});

	it('rejects the /\\host backslash trick', () => {
		expect(safeReturnTo('/\\evil.example')).toBeNull();
	});

	it('rejects a non-path (no leading slash) and empty/null', () => {
		expect(safeReturnTo('reserve/abc')).toBeNull();
		expect(safeReturnTo('')).toBeNull();
		expect(safeReturnTo(null)).toBeNull();
		expect(safeReturnTo(undefined)).toBeNull();
	});
});
