import createClient from 'openapi-fetch';
import type { paths } from './schema';

// Typed API client generated from api/openapi.yaml (ADR-0006 §3): request/response
// types come from ./schema and are never hand-written (regenerated + drift-guarded
// in CI). Base URL is resolved by the deployment; same-origin by default. Feature
// cuts (F2+) add server-side auth + Host forwarding — F1 only proves the wiring.
export const api = createClient<paths>({ baseUrl: '/' });
