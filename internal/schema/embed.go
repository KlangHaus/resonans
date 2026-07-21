package schema

import (
	"embed"
	"io/fs"
)

// schemaFiles embeds the code-defined content schema (block library + doc-types)
// so the binary carries it without a filesystem dependency (distroless).
//
//go:embed schemas
var schemaFiles embed.FS

// DefaultRegistry loads the embedded schema.
func DefaultRegistry() (*Registry, error) {
	sub, err := fs.Sub(schemaFiles, "schemas")
	if err != nil {
		return nil, err
	}
	return Load(sub)
}
