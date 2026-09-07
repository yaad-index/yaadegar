export const base = '';
export const assets = '';

// SvelteKit's resolve() substitutes route parameters — resolve('/l/[shareSlug]', {
// shareSlug: 'abc' }) is '/l/abc'. The stub used to return the route id unchanged,
// which made every parameterised href untestable: an assertion about where a link
// actually points would either fail against the literal '[param]' or, if written
// loosely enough to pass, assert nothing. Substituting here lets a test say what it
// means. Routes without parameters are unaffected, which is every existing caller.
const substitute = (route: string, params: Record<string, string>) =>
	route.replace(/\[(?:\.{3})?([^\]]+)\]/g, (match, name: string) => params[name] ?? match);

export const resolve = (p: string, params?: Record<string, string>) =>
	params ? substitute(p, params) : p;
export const resolveRoute = resolve;
