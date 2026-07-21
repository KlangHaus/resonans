# Multi-stage build for the Resonans CMS (Go REST content-API).
# Mirrors the permissions/notifications service Dockerfiles (Docker Compose /
# Caddy deploy model per docs/infrastructure — not k8s).
FROM golang:1.26-alpine AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X github.com/KlangHaus/resonans/internal/app.version=${VERSION}" \
    -o /out/resonans ./cmd/resonans

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/resonans /app/resonans
# Migrations bundled as immutable deploy artifacts (also run at boot from embed).
COPY --from=build /src/migrations /app/migrations
USER nonroot:nonroot
# Content-API port. Caddy + Compose reverse-proxy to this (vm2-ops Caddyfile:
# reverse_proxy resonans:3000). Deploy contract — do not change without an
# accompanying infra/compose change.
EXPOSE 3000
# Distroless ships no shell or wget/curl, so the healthcheck invokes the binary
# itself in `healthcheck` mode (probes /healthz on the configured PORT).
HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=5 \
    CMD ["/app/resonans", "healthcheck"]
ENTRYPOINT ["/app/resonans"]
