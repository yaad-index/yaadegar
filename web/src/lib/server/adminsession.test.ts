import { describe, it, expect } from 'vitest';
import type { Cookies } from '@sveltejs/kit';
import { ADMIN_SESSION_COOKIE, readAdminSession } from './adminsession';
import { SESSION_COOKIE } from './session';

describe('admin session', () => {
	it('uses a cookie name distinct from the owner session', () => {
		// The admin surface must never share a cookie with the owner session, or an
		// admin token could be read as an owner token (ADR-0009 Cut 1b).
		expect(ADMIN_SESSION_COOKIE).toBe('yaadegar_admin_session');
		expect(ADMIN_SESSION_COOKIE).not.toBe(SESSION_COOKIE);
	});

	it('reads only its own cookie, not the owner cookie', () => {
		const cookies = {
			get: (name: string) => (name === ADMIN_SESSION_COOKIE ? 'admin.jwt' : undefined)
		} as unknown as Cookies;
		expect(readAdminSession(cookies)).toBe('admin.jwt');

		const ownerOnly = {
			get: (name: string) => (name === SESSION_COOKIE ? 'owner.jwt' : undefined)
		} as unknown as Cookies;
		// An owner cookie must NOT satisfy the admin session read.
		expect(readAdminSession(ownerOnly)).toBeUndefined();
	});
});
