# Multi-stage build for the Resonans CMS (Go REST content-API).
# Mirrors the permissions/notifications service Dockerfiles (Docker Compose /
# Caddy deploy model per docs/infrastructure — not k8s).
# Pinned to the exact patch in go.mod's `go` directive: CI runs tests on the
# directive version, so a floating tag would ship a stdlib we never tested
# against ([permissions]-fund 2026-08-14). Held at 1.25 because the
# golangci-lint v2.5.0 pin in ci-go.yml is built with go1.25 and refuses a
# 1.26 directive (målt på denne PR: exit code 3). Bump both together.
FROM golang:1.25.13-alpine AS build
ARG VERSION=dev
# REVISION = full git sha. Stamped into the binary so /healthz can report which
# build is running — the OCI label alone is invisible to a running process.
ARG REVISION=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X github.com/KlangHaus/resonans/internal/app.version=${VERSION} -X github.com/KlangHaus/resonans/internal/app.revision=${REVISION}" \
    -o /out/resonans ./cmd/resonans

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/resonans /app/resonans
# Migrations bundled as immutable deploy artifacts (also run at boot from embed).
COPY --from=build /src/migrations /app/migrations
USER nonroot:nonroot
# Content-API port. A reverse proxy forwards to this. Deploy contract — do not
# change it without changing the deployment that forwards here; compose.yml in
# this repository is the reference.
EXPOSE 3000
# Distroless ships no shell or wget/curl, so the healthcheck invokes the binary
# itself in `healthcheck` mode (probes /healthz on the configured PORT).
HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=5 \
    CMD ["/app/resonans", "healthcheck"]
ENTRYPOINT ["/app/resonans"]
