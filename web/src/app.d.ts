// See https://svelte.dev/docs/kit/types#app.d.ts
declare global {
	namespace App {
		// interface Error {}
		interface Locals {
			/** Tenant host for backend Host-based routing. */
			host: string;
			/** Owner session JWT from the httpOnly cookie, if present. */
			token?: string;
			/** Instance-admin (superadmin) session JWT from a SEPARATE httpOnly cookie,
			 * if present. Read only by the /admin surface; never mixed with the owner
			 * token (ADR-0009 Cut 1b). */
			adminToken?: string;
		}
		// interface PageData {}
		// interface PageState {}
		// interface Platform {}
	}
}

export {};
