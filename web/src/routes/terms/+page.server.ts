import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';
import { resolveLegalPage } from '$lib/legal';

// /terms (#300). Same shape as /privacy: unauthenticated, no tenant requirement, resolving
// on every host the web service answers on. It renders the installation's text when
// YAADEGAR_TERMS_TEXT is set and an explicit PLACE HOLDER when it is not; the project ships
// no default legal wording (lib/legal.ts).
export const load: PageServerLoad = () => ({ page: resolveLegalPage('terms', env) });
