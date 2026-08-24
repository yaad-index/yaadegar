// Stub for SvelteKit's $env/dynamic/private in the component / server-logic test
// project. The kit plugin provides this virtual module at runtime; here a plain mutable
// record stands in, so a server module that reads it resolves. Unlike the $app/* stubs,
// this one IS read (the root-page loader passes it to resolveRootPage), so it starts
// empty — which resolves to the bundled default — and a test can set keys on it and
// clear them afterwards to exercise the login/custom modes.
export const env: Record<string, string | undefined> = {};
