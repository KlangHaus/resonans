// Package content is the Resonans CMS domain: tenant-scoped content documents,
// their storage contract, and the two Store implementations (Postgres+RLS for
// real deploys, in-memory for dev/test). It has no HTTP knowledge.
//
// Every operation is scoped to a tenant (tenant_id ≡ org_id, Beslutning K): the
// tenantID is passed explicitly by the caller (derived from the verified JWT
// actor, never from request body/header) so cross-tenant access is impossible.
// The Postgres store additionally enforces this in the database via RLS.
package content

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// DefaultDataset is the content space used until dataset-switching lands (Fase 1+).
const DefaultDataset = "production"

// Status values for a document's publication lifecycle.
const (
	StatusDraft     = "draft"
	StatusPublished = "published"
)

// Sentinel errors; httpapi maps these to status codes (404/409/400).
var (
	ErrNotFound     = errors.New("content: document not found")
	ErrSlugConflict = errors.New("content: slug already exists for (tenant, dataset, type)")
	ErrInvalid      = errors.New("content: invalid input")
)

// Document is a single piece of content, scoped to a tenant + dataset. Its
// `Type` (course, lesson, page, post, ...) and JSON `Body` are validated against
// the schema-registry in the application layer (Fase 1); at this layer a
// document is a typed, tenant-owned JSONB record.
type Document struct {
	ID          uuid.UUID       `json:"id"`
	TenantID    uuid.UUID       `json:"tenantId"`
	Dataset     string          `json:"dataset"`
	Type        string          `json:"type"`
	Slug        string          `json:"slug"`
	Title       string          `json:"title"`
	Body        json.RawMessage `json:"body"`
	Status      string          `json:"status"`
	Version     int             `json:"version"`
	PublishedAt *time.Time      `json:"publishedAt,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// ListFilter narrows a List query within a tenant. Empty fields are ignored.
type ListFilter struct {
	Type   string
	Status string
}

// CreateInput is the payload to create a new (draft) document.
type CreateInput struct {
	Type  string
	Slug  string
	Title string
	Body  json.RawMessage
}

// Validate checks required fields and normalises the body to non-nil JSON.
func (in *CreateInput) Validate() error {
	if in.Type == "" || in.Slug == "" || in.Title == "" {
		return ErrInvalid
	}
	if len(in.Body) == 0 {
		in.Body = json.RawMessage(`{}`)
	}
	if !json.Valid(in.Body) {
		return ErrInvalid
	}
	return nil
}

// UpdateInput is a partial update; nil fields are left unchanged.
type UpdateInput struct {
	Type  *string
	Title *string
	Body  *json.RawMessage
}

// Validate rejects an update whose provided body is not valid JSON.
func (in *UpdateInput) Validate() error {
	if in.Body != nil && !json.Valid(*in.Body) {
		return ErrInvalid
	}
	return nil
}

// Store is the persistence contract. Every method is tenant-scoped: tenantID is
// the authoritative tenant (from the verified actor) and callers must never
// pass a tenant sourced from request input.
type Store interface {
	List(ctx context.Context, tenantID uuid.UUID, f ListFilter) ([]Document, error)
	Get(ctx context.Context, tenantID, id uuid.UUID) (Document, error)
	// GetPublished returns the published document for (type, slug) in the
	// tenant's default dataset — the read path consumed by our Nuxt sites.
	GetPublished(ctx context.Context, tenantID uuid.UUID, typ, slug string) (Document, error)
	Create(ctx context.Context, tenantID uuid.UUID, in CreateInput) (Document, error)
	Update(ctx context.Context, tenantID, id uuid.UUID, in UpdateInput) (Document, error)
	Publish(ctx context.Context, tenantID, id uuid.UUID) (Document, error)
}
