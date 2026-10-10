# Resonans Studio — Nuxt SPA served by the Nitro node-server, multi-stage.
# The node-server (not a static file host) matters: it injects NUXT_PUBLIC_*
# runtime config (apiBase, authBase) into the served app at RUNTIME, so one
# image works across environments.
# 🔴 PINNED AS `tag@digest`. The digest fixes the bytes; the TAG is what
# Dependabot tracks, and without it a lift proposes `latest` — for this repo a
# different Node major (measured: `node:latest` is sha256:32fa97f3…, not our
# 22-slim). See the Dockerfile at the repository root for the full reasoning
# and the #114 measurement that produced it.
#
# This one was found by enumerating EVERY image reference in the repo rather
# than the two the task named: `:22-slim` floats across 22.x patches, and this
# image is what studio actually ships. Watched by the `/studio` entry in
# .github/dependabot.yml, which exists because a pin nobody lifts is a tag that
# stopped floating AND stopped getting security fixes.
#
# 🔴 BOTH stages carry the same digest on purpose: a build stage on one Node and
# a runtime stage on another is a difference that would not show up in any test
# here, because the gate builds and typechecks but runs nothing at runtime.
FROM node:22-slim@sha256:c3de60bf2f9dd0ac6370e6117950ff62d6e339527e7472301c9c78a017978392 AS build
WORKDIR /app
ENV PNPM_HOME=/pnpm PATH=/pnpm:$PATH
RUN corepack enable
# No glob on the lockfile: `pnpm-lock.yaml*` copies nothing when the file is
# missing, and the build then resolves versions from scratch without anyone
# noticing that the reviewed lockfile was never present.
COPY package.json pnpm-lock.yaml ./
# `|| pnpm install` used to follow. It turned a lockfile that disagrees with
# package.json into a GREEN build on versions nobody reviewed — and it only
# ever ran when something was already wrong, which is the worst possible
# trigger condition for a fallback. Concretely: it could have re-resolved the
# vulnerable transitive dependency that #70 had just removed, and the build
# would have reported success. A disagreement must stop the build.
RUN pnpm install --frozen-lockfile
COPY . .
RUN pnpm build

FROM node:22-slim@sha256:c3de60bf2f9dd0ac6370e6117950ff62d6e339527e7472301c9c78a017978392 AS runtime
WORKDIR /app
# Build identity, surfaced at /healthz. Kept as ENV (not runtimeConfig) so the
# value is readable at request time — runtimeConfig would bake it at build time.
ARG VERSION=dev
ARG REVISION=unknown
ENV NODE_ENV=production HOST=0.0.0.0 PORT=3000 \
    RESONANS_STUDIO_VERSION=${VERSION} \
    RESONANS_STUDIO_REVISION=${REVISION}
# Run as non-root.
USER node
COPY --from=build --chown=node:node /app/.output ./.output
EXPOSE 3000
# Probe /healthz, not /: the SPA shell answers 200 even when the Nitro server
# routes are broken, so / cannot distinguish "serving" from "serving nothing".
HEALTHCHECK --interval=15s --timeout=4s --retries=5 \
  CMD node -e "fetch('http://127.0.0.1:3000/healthz').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"
CMD ["node", ".output/server/index.mjs"]
