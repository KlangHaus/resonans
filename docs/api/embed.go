// Package apispec embeds the OpenAPI document so the binary can serve it
// without a filesystem dependency -- the runtime image is distroless and has
// no shell, and `go run ./cmd/resonans` has no image at all.
//
// It lives NEXT TO the specification rather than copying it into a Go package,
// because `go:embed` cannot reach a parent directory and a second copy of a
// 16 KB document is a drift hazard: the cell in internal/apidocs would keep
// one copy honest while the served one went stale. Same shape as
// migrations/embed.go.
package apispec

import _ "embed"

// Document is docs/api/openapi.yaml, the single source the drift cell in
// internal/apidocs compares against the registered routes.
//
//go:embed openapi.yaml
var Document []byte
