import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';
import { resolveLegalPage } from '$lib/legal';

// /privacy (#300). A top-level route with no auth gate and no tenant requirement, so it
// resolves unauthenticated on every host the web service answers on — which is what lets
// Google fetch it while verifying the OAuth consent screen. It renders the installation's
// text when YAADEGAR_PRIVACY_TEXT is set and an explicit PLACE HOLDER when it is not; the
// project ships no default legal wording (lib/legal.ts).
export const load: PageServerLoad = () => ({ page: resolveLegalPage('privacy', env) });
