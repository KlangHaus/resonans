// Package settings holds per-tenant configuration — first up: the content
// profile (which doc-types a tenant has enabled). An empty enabled-set means
// ALL types are enabled (backwards compatible: tenants without a settings row
// see the full registry). Tenant-scoped like content: the caller passes the
// verified tenant, and the Postgres store enforces RLS via the GUC.
package settings

import (
	"context"
	"errors"
	"net/url"
	"strings"

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
	// PreviewURL returns the tenant's preview URL template ("" = not set).
	PreviewURL(ctx context.Context, tenantID uuid.UUID) (string, error)
	// SetPreviewURL replaces the template. Callers validate it first
	// (ValidatePreviewURL); "" turns visual editing off.
	SetPreviewURL(ctx context.Context, tenantID uuid.UUID, template string) error
}

// ErrInvalidPreviewURL is returned by ValidatePreviewURL.
var ErrInvalidPreviewURL = errors.New("invalid preview url")

// ValidatePreviewURL checks a preview URL template and returns its origin — the
// ONLY origin the Studio will exchange postMessages with. The template must be
// an absolute https URL (plain http only for localhost, for local development)
// and must carry the {token} placeholder: without the token the frontend cannot
// read the draft, and a template that silently renders published content would
// look like a working preview while showing the wrong revision.
func ValidatePreviewURL(template string) (string, error) {
	if !strings.Contains(template, "{token}") {
		return "", ErrInvalidPreviewURL
	}
	// The origin must be fixed: no placeholder before the path. Checked on the
	// RAW template — after filling, "{slug}.example" is an ordinary host.
	if _, rest, ok := strings.Cut(template, "://"); ok {
		authority := rest
		if i := strings.IndexAny(rest, "/?#"); i >= 0 {
			authority = rest[:i]
		}
		if strings.ContainsAny(authority, "{}") {
			return "", ErrInvalidPreviewURL
		}
	}
	// Placeholders are not valid URL characters everywhere; parse a filled copy.
	filled := strings.NewReplacer("{type}", "t", "{slug}", "s", "{token}", "x", "{locale}", "da").Replace(template)
	u, err := url.Parse(filled)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" {
		return "", ErrInvalidPreviewURL
	}
	switch u.Scheme {
	case "https":
	case "http":
		if h := u.Hostname(); h != "localhost" && h != "127.0.0.1" {
			return "", ErrInvalidPreviewURL
		}
	default:
		return "", ErrInvalidPreviewURL
	}
	return u.Scheme + "://" + u.Host, nil
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
