// Package settings holds per-tenant configuration — first up: the content
// profile (which doc-types a tenant has enabled). An empty enabled-set means
// ALL types are enabled (backwards compatible: tenants without a settings row
// see the full registry). Tenant-scoped like content: the caller passes the
// verified tenant, and the Postgres store enforces RLS via the GUC.
package settings

import (
	"context"

	"github.com/google/uuid"
)

// Store persists per-tenant settings.
type Store interface {
	// EnabledTypes returns the tenant's enabled doc-type names. Empty slice =
	// all types enabled (no row / explicit "everything").
	EnabledTypes(ctx context.Context, tenantID uuid.UUID) ([]string, error)
	// SetEnabledTypes replaces the tenant's enabled doc-type set. An empty
	// slice re-enables everything.
	SetEnabledTypes(ctx context.Context, tenantID uuid.UUID, types []string) error
}

// Enabled reports whether typ is enabled given an enabled-set (empty = all).
func Enabled(enabledTypes []string, typ string) bool {
	if len(enabledTypes) == 0 {
		return true
	}
	for _, t := range enabledTypes {
		if t == typ {
			return true
		}
	}
	return false
}
