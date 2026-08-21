# Multi-stage: build the SvelteKit adapter-node app, then run it on a slim Node
# runtime carrying production dependencies only. The generated OpenAPI client
# (src/lib/api/schema.d.ts) is committed and drift-guarded, so the build needs no
# access to the OpenAPI spec.

# --- builder ---
FROM node:20-bookworm-slim AS build
WORKDIR /app
# Install against the lockfile first so this layer caches across source changes.
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
# Build the node-adapter server, then drop dev dependencies so only the runtime
# set is carried forward.
RUN npm run build && npm prune --omit=dev

# --- runtime ---
FROM node:20-bookworm-slim
WORKDIR /app
ENV NODE_ENV=production
ENV PORT=3000
ENV HOST=0.0.0.0
# Build version, stamped in at build time and reported by the running server
# (ADR-0014 §2), mirroring the API's -X main.version link-time stamp. The publish
# workflow passes the resolved release version as this build-arg; a source or local
# build (docker compose, contributor path) leaves it "dev", which the skew check
# treats as unstamped rather than as a mismatched release. Not "" — an empty default
# would make a dropped build-arg indistinguishable from a real build in the
# round-trip check, the same trap the API's default guards against.
ARG VERSION=dev
ENV YAADEGAR_VERSION=${VERSION}
# The adapter-node server, the pruned production deps, and the manifest it reads.
COPY --from=build /app/build ./build
COPY --from=build /app/node_modules ./node_modules
COPY --from=build /app/package.json ./
EXPOSE 3000
USER node
CMD ["node", "build/index.js"]
