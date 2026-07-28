# Yaadegar web

The Yaadegar frontend — SvelteKit + TypeScript (ADR-0006). This is **Cut F1**:
foundations only (framework, generated API client, the library baseline, CI, one
placeholder page). No owner/giver features yet — those land in later cuts.

## Stack

- **SvelteKit** (Svelte 5, runes) + **TypeScript**, node adapter.
- **Typed API client generated from `../api/openapi.yaml`** via `openapi-typescript`
  (types) + `openapi-fetch` (requests) — never hand-written; CI fails on drift.
- **TanStack Query** (Svelte adapter), **sveltekit-superforms** + **zod**, **bits-ui**
  components, **Tailwind CSS**.

## Develop

Requires **Node 20** (`.nvmrc`); `nvm use`.

```sh
npm install
npm run dev            # dev server
npm run generate:api   # regenerate src/lib/api/schema.d.ts from the spec
npm run check          # typecheck (svelte-check)
npm run lint           # prettier + eslint
npm run build          # production build (node adapter)
```

`npm run verify:api` regenerates the client and fails if it differs from the
checked-in `src/lib/api/schema.d.ts` — the drift-guard CI runs.
