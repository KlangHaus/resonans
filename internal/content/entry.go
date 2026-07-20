// Package content is the Resonans CMS domain: content entries, their storage
// contract, and the two Store implementations (Postgres for real deploys,
// in-memory for dev/test). It has no HTTP knowledge — internal/httpapi adapts
// it to REST (Beslutning W: content-API to our own Nuxt sites = REST).
package content

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Status values for an entry's publication lifecycle.
const (
	StatusDraft     = "draft"
	StatusPublished = "published"
)

// Sentinel errors the Store implementations return; httpapi maps these to
// status codes (404/409/400).
var (
	// ErrNotFound is returned when no entry matches the lookup.
	ErrNotFound = errors.New("content: entry not found")
	// ErrSlugConflict is returned when a (site, slug) pair already exists.
	ErrSlugConflict = errors.New("content: slug already exists for site")
	// ErrInvalid is returned when input fails validation.
	ErrInvalid = errors.New("content: invalid input")
)

// Entry is a single piece of content, scoped to a site.
type Entry struct {
	ID          uuid.UUID       `json:"id"`
	Site        string          `json:"site"`
	Type        string          `json:"type"`
	Slug        string          `json:"slug"`
	Title       string          `json:"title"`
	Body        json.RawMessage `json:"body"`
	Status      string          `json:"status"`
	PublishedAt *time.Time      `json:"publishedAt,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// ListFilter narrows a List query. Empty fields are ignored (no filter).
type ListFilter struct {
	Site   string
	Type   string
	Status string
}

// CreateInput is the payload to create a new (draft) entry.
type CreateInput struct {
	Site  string
	Type  string
	Slug  string
	Title string
	Body  json.RawMessage
}

// Validate checks required fields and normalises the body to a non-nil JSON.
func (in *CreateInput) Validate() error {
	if in.Site == "" || in.Type == "" || in.Slug == "" || in.Title == "" {
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

// Store is the persistence contract for content entries.
type Store interface {
	List(ctx context.Context, f ListFilter) ([]Entry, error)
	Get(ctx context.Context, id uuid.UUID) (Entry, error)
	// GetPublished returns the published entry for (site, slug) — the public
	// read path consumed by our Nuxt sites. Draft entries are treated as not
	// found here.
	GetPublished(ctx context.Context, site, slug string) (Entry, error)
	Create(ctx context.Context, in CreateInput) (Entry, error)
	Update(ctx context.Context, id uuid.UUID, in UpdateInput) (Entry, error)
	Publish(ctx context.Context, id uuid.UUID) (Entry, error)
}
