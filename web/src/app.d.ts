// See https://svelte.dev/docs/kit/types#app.d.ts
declare global {
	namespace App {
		// interface Error {}
		interface Locals {
			/** Tenant host for backend Host-based routing. */
			host: string;
			/** Owner session JWT from the httpOnly cookie, if present. The /admin surface
			 * reuses this same owner session — admin is a capability on the owner account
			 * (ADR-0010), not a separate identity. */
			token?: string;
		}
		// interface PageData {}
		// interface PageState {}
		// interface Platform {}
	}
}

export {};
