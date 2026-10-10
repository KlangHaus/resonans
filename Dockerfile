# Multi-stage build for the Resonans CMS (Go REST content-API).
# Mirrors the permissions/notifications service Dockerfiles (Docker Compose /
# Caddy deploy model per docs/infrastructure — not k8s).
# Pinned to the exact patch in go.mod's `go` directive: CI runs tests on the
# directive version, so a floating tag would ship a stdlib we never tested
# against ([permissions]-fund 2026-08-14). Held at 1.25 because the
# golangci-lint v2.5.0 pin in ci-go.yml is built with go1.25 and refuses a
# 1.26 directive (målt på denne PR: exit code 3). Bump both together.
# 🔴 PINNED AS `tag@digest`, AND THE TAG IS LOAD-BEARING — not decoration.
#
# A tag is a promise about an object; only the digest IS the object. So the
# digest is what pins the bytes. But the TAG is what Dependabot tracks, and
# #110 shipped these as digest ONLY. Measured consequence, within minutes:
# Dependabot opened #114 proposing distroless `afa5c872…` -> `d75cdd72…`, and
# one layer below the platform manifest both have the SAME 12 layers with an
# identical layer signature — only the config differed, `User` 65532 -> 0 and
# `WorkingDir` /home/nonroot -> /. Zero new content: it was a silent switch
# from the `:nonroot` variant to the `:latest` one, because a digest with no
# tag leaves `latest` as the only thing to track.
#
# 🔴 So a lifting mechanism is not enough; it has to lift toward the RIGHT
# object. With the tag present, Dependabot lifts WITHIN `1.26.9-alpine` and the
# digest still fixes the bytes. #114 was closed rather than merged.
#
# Digests resolved 2026-10-10 by this repository's own registry requests, each
# with an invented tag returning 404 and no digest as the control. The Scalar
# renderer below has been SHA-pinned since it was added.
#
# 🔴 AND A MANIFEST DIGEST IS NOT CONTENT. It changes with the config too. When
# a lift arrives, compare LAYERS — not the manifest digest, and not the diff.
#
# 🔴 REVISIT when the go directive leaves 1.26: the digest, the tag in this
# comment and go.mod must move together, and the pinned gosec in ci-go.yml must
# still be able to read what the new toolchain emits.
FROM golang:1.26.9-alpine@sha256:cdfd4fe2da6b225d8b40c6b7a105736e548e83ff56d5d8f9394446eeb5eb84e0 AS build
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

# ─── API-reference renderer ──────────────────────────────────────────────────
# Fetched HERE rather than vendored into git. Measured 2026-10-02: the bundle is
# 4 381 105 bytes raw, and over 1.2 MB gzipped, against a .git of 4.2 MB whose
# largest tracked file is 244 KB — so vendoring would be a 29 % permanent
# increase in a history that is about to become public. Fetching at IMAGE build
# time keeps `go build` offline and reproducible (the binary does not need the
# asset) and puts the download where a sha256 pin can be verified.
#
# The same golang image as the build stage, deliberately: it is already pinned
# and already has wget, so this adds no second base image to keep fresh.
#
# 🔴 FAILS CLOSED. If the download fails or the digest does not match, the RUN
# fails and no image is produced. The alternative — skipping the asset — would
# build an image whose documentation page degrades exactly like a development
# binary, which is the one failure that would look normal.
#
# scripts/apidocs_pin_guard.py reads these two ARGs as the single source of
# truth and compares SCALAR_VERSION against the registry's dist-tags.latest.
#
# 🔴 DO NOT COMPARE COMPRESSED BYTE COUNTS. The gzipped size depends on which
# gzip ran, and an earlier version of this comment recorded one machine's
# figure, which sent a reader chasing a 1 375-byte "drift" that was not one.
# Measured 2026-10-03 on byte-identical input: python zlib -9 gives 1 263 386,
# macOS gzip -9 gives 1 263 396, busybox gzip -9 in this stage gives
# 1 264 771, python level 6 gives 1 266 487 — a 3 101-byte spread.
#
# The reproducible invariants are the RAW sha256 pinned below and the
# decompressed size (4 381 105). Both are verified in the build: the digest by
# sha256sum, which logs `/tmp/renderer.js: OK`, and the archive by `gzip -t`.
FROM golang:1.26.9-alpine@sha256:cdfd4fe2da6b225d8b40c6b7a105736e548e83ff56d5d8f9394446eeb5eb84e0 AS apidocs
ARG SCALAR_VERSION=1.72.4
ARG SCALAR_SHA256=f5ac0c3504a7ca77ca9fa96a6bb7c8f98b42c271864df35583f6ed9e6da40c23
# 🔴 THE PIN'S FORMAT IS ASSERTED BEFORE IT IS USED, and that is not belt and
# braces. Measured 2026-10-02: `echo "deadbeef  file" | sha256sum -c -` prints
# "WARNING: 1 line is improperly formatted" and exits 0 -- a MALFORMED digest
# PASSES. So a truncated or mistyped SCALAR_SHA256 would silently verify
# nothing. That was measured with GNU coreutils; busybox's sha256sum, which is
# what actually runs here, could NOT be measured (no Docker daemon available at
# the time of writing), so this does not rely on either one's behaviour: a pin
# that is not exactly 64 lowercase hex characters fails the build on its own.
RUN mkdir -p /out \
 && printf '%s' "${SCALAR_SHA256}" | grep -Eq '^[0-9a-f]{64}$' \
      || { echo "SCALAR_SHA256 is not 64 lowercase hex characters: refusing to verify nothing"; exit 1; }
RUN wget -q -O /tmp/renderer.js \
      "https://cdn.jsdelivr.net/npm/@scalar/api-reference@${SCALAR_VERSION}/dist/browser/standalone.js" \
 && echo "${SCALAR_SHA256}  /tmp/renderer.js" | sha256sum -c - \
 && gzip -9 -c /tmp/renderer.js > /out/renderer.js.gz \
 && gzip -t /out/renderer.js.gz

# 🔴 THE RUNTIME BASE, and the one that actually SHIPS — pinned last and found
# late: this change began as "two pins" and the enumeration of the whole path
# turned up three. `nonroot` is a moving tag on distroless, so the image running
# in production was the least pinned of the three.
#
# `gcr.io/distroless/static-debian12@sha256:afa5c872…` IS `:nonroot`, resolved
# 2026-10-10 from gcr.io by this repository's own request, with an invented tag
# returning 404 and no digest as the control.
#
# Watched by the `docker` ecosystem in .github/dependabot.yml together with the
# golang stages above; the same REVISIT note applies.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
WORKDIR /app
COPY --from=build /out/resonans /app/resonans
# The renderer, pre-gzipped. internal/apiref serves it with Content-Encoding:
# gzip, so these bytes are never decompressed on our side at all.
COPY --from=apidocs /out/renderer.js.gz /app/apidocs/renderer.js.gz
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
