// Package migrations embeds the SQL migration files so the binary can run them
// at boot without a filesystem dependency (distroless has no shell). The same
// .sql files are also COPYied into the image for ops inspection.
package migrations

import "embed"

// FS holds all *.sql migrations in lexical filename order.
//
//go:embed *.sql
var FS embed.FS
