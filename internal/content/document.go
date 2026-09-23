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
	"fmt"
	"regexp"
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
	// ErrInvalidSlug wraps ErrInvalid (still a 400) with the reason.
	ErrInvalidSlug = fmt.Errorf("%w: slug must be lowercase a-z, 0-9 and single hyphens (1-200 chars)", ErrInvalid)
	// ErrDeleted is returned when an operation is attempted on a document that
	// is in the bin. It is NOT ErrNotFound: the editor must be able to tell
	// "this is gone" from "this is deleted, restore it first", or they will
	// recreate content that is one click from coming back.
	ErrDeleted = errors.New("content: document is deleted; restore it first")
	// ErrNotDeleted is the restore of a document that was never deleted — a
	// no-op dressed as an action, which hides a mistaken id.
	ErrNotDeleted = errors.New("content: document is not deleted")
)

// slugPattern is the only slug shape stored. Frontends build links as
// "/" + slug: a slug starting with "/" or "\\" made "//host" — an off-site
// link on blog (measured, riff re6owzgdd7vbkxhbvqb0rqok). It is exactly what the
// Studio's slugify produces, and every seed and fixture slug matches it.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidSlug reports whether s is a storable slug.
func ValidSlug(s string) bool { return len(s) <= 200 && slugPattern.MatchString(s) }

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
	// DeletedAt is nil for a live document. It is omitted from JSON when nil,
	// so a client that does not know about the bin cannot mistake a live
	// document for a deleted one — and a client that does know gets the fact
	// rather than having to infer it from an absence.
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
}

// IsDeleted reports whether the document is in the bin.
func (d Document) IsDeleted() bool { return d.DeletedAt != nil }

// ListFilter narrows a List query within a tenant. Empty fields are ignored.
type ListFilter struct {
	Type   string
	Status string
	// Deleted selects which side of the bin to list. The zero value excludes
	// deleted documents, so every existing caller keeps its meaning and no
	// deleted document can appear in a listing by accident.
	Deleted DeletedFilter
}

// DeletedFilter is a three-state selector; the zero value is the safe one.
type DeletedFilter int

const (
	// DeletedExclude lists only live documents (default).
	DeletedExclude DeletedFilter = iota
	// DeletedOnly lists only the bin.
	DeletedOnly
	// DeletedInclude lists both, and is used by nothing on the delivery path.
	DeletedInclude
)

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
	if !ValidSlug(in.Slug) {
		return ErrInvalidSlug
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

// Version is an append-only snapshot of a document (written on publish) for
// history + rollback (Fase 5 workflow).
type Version struct {
	ID         uuid.UUID       `json:"id"`
	DocumentID uuid.UUID       `json:"documentId"`
	Version    int             `json:"version"`
	Title      string          `json:"title"`
	Body       json.RawMessage `json:"body"`
	Status     string          `json:"status"`
	CreatedAt  time.Time       `json:"createdAt"`
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
	// GetBySlug returns the document for (type, slug) regardless of status — the
	// preview (draft-read) path.
	GetBySlug(ctx context.Context, tenantID uuid.UUID, typ, slug string) (Document, error)
	Create(ctx context.Context, tenantID uuid.UUID, in CreateInput) (Document, error)
	Update(ctx context.Context, tenantID, id uuid.UUID, in UpdateInput) (Document, error)
	Publish(ctx context.Context, tenantID, id uuid.UUID) (Document, error)

	// ListVersions returns a document's snapshots, newest first.
	ListVersions(ctx context.Context, tenantID, docID uuid.UUID) ([]Version, error)
	// Rollback restores a prior version's content as a new draft.
	Rollback(ctx context.Context, tenantID, docID uuid.UUID, version int) (Document, error)

	// SoftDelete puts a document in the bin: the row, its versions and its
	// history stay, and it disappears from listings, delivery and preview.
	// Deleting an already-deleted document is ErrDeleted rather than a silent
	// success, so a double click is visible.
	SoftDelete(ctx context.Context, tenantID, id uuid.UUID) (Document, error)
	// Restore takes a document back out of the bin. It fails with
	// ErrSlugConflict when a live document has taken the slug in the meantime
	// — the live one keeps it, and the editor is told which one collides.
	Restore(ctx context.Context, tenantID, id uuid.UUID) (Document, error)
}
