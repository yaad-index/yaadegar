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
			/** The client's address as this server sees it (getClientAddress, which honours
			 * ADDRESS_HEADER behind a reverse proxy). Forwarded to the backend as
			 * X-Forwarded-For so its rate limits key on the real client. Absent when the
			 * adapter cannot report one. */
			clientIP?: string;
		}
		// interface PageData {}
		// interface PageState {}
		// interface Platform {}
	}
}

export {};
