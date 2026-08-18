# Resonans Studio — Nuxt SPA served by the Nitro node-server, multi-stage.
# Same VM2-ops pattern as the blog image. The node-server (not a static file
# host) matters: it injects NUXT_PUBLIC_* runtime config (apiBase, devTenant)
# into the served app at RUNTIME, so one image works across environments.
FROM node:22-slim AS build
WORKDIR /app
ENV PNPM_HOME=/pnpm PATH=/pnpm:$PATH
RUN corepack enable
COPY package.json pnpm-lock.yaml* ./
RUN pnpm install --frozen-lockfile || pnpm install
COPY . .
RUN pnpm build

FROM node:22-slim AS runtime
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
