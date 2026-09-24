// Package media is the Resonans asset pipeline (Fase 3): tenant-scoped media
// assets stored in object storage (Bunny) + referenced from content by id, plus
// a pluggable video-provider (Vimeo) for video-embed blocks.
//
// Like content, every asset is tenant-scoped (Beslutning K): the tenantID is
// passed explicitly by the caller (from the verified actor) and the Postgres
// store enforces isolation via RLS.
package media

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Kinds of stored asset.
const (
	KindImage = "image"
	KindFile  = "file"
)

// ErrNotFound is returned when no asset matches the lookup.
var ErrNotFound = errors.New("media: asset not found")

// ErrDeleted and ErrNotDeleted separate "in the bin" from "never existed" and
// from "already live". They are distinct from ErrNotFound on purpose: an editor
// who cannot find their file needs to know which of those three happened, and a
// 404 for a deleted asset would send them looking for an upload that is one
// click from coming back.
var (
	ErrDeleted    = errors.New("media: asset is deleted")
	ErrNotDeleted = errors.New("media: asset is not deleted")
)

// Asset is a stored media file's metadata. The bytes live in object storage
// under StorageKey; content references the asset by ID.
type Asset struct {
	ID         uuid.UUID       `json:"id"`
	TenantID   uuid.UUID       `json:"tenantId"`
	Kind       string          `json:"kind"`
	StorageKey string          `json:"storageKey"`
	Filename   string          `json:"filename"`
	Mime       string          `json:"mime"`
	SizeBytes  int64           `json:"sizeBytes"`
	Width      *int            `json:"width,omitempty"`
	Height     *int            `json:"height,omitempty"`
	Hotspot    json.RawMessage `json:"hotspot"`
	CreatedAt  time.Time       `json:"createdAt"`
	// DeletedAt is the bin. Omitted when nil so a live asset's JSON is unchanged
	// for every existing consumer.
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
}

// IsDeleted reports whether the asset is in the bin.
func (a Asset) IsDeleted() bool { return a.DeletedAt != nil }

// DeletedFilter selects which side of the bin List returns. Mirrors
// content.DeletedFilter, including that the zero value excludes the bin: a
// caller that forgets to set it gets live assets, never deleted ones.
type DeletedFilter int

const (
	// DeletedExclude lists only live assets (default).
	DeletedExclude DeletedFilter = iota
	// DeletedOnly lists only the bin.
	DeletedOnly
	// DeletedInclude lists both.
	DeletedInclude
)

// ListFilter narrows a listing.
type ListFilter struct {
	Deleted DeletedFilter
}

// Store is the tenant-scoped persistence contract for asset metadata.
type Store interface {
	Create(ctx context.Context, tenantID uuid.UUID, a Asset) (Asset, error)
	Get(ctx context.Context, tenantID, id uuid.UUID) (Asset, error)
	List(ctx context.Context, tenantID uuid.UUID, f ListFilter) ([]Asset, error)
	// SoftDelete moves an asset to the bin and returns it. The stored bytes are
	// NOT touched — see migrations/0009.
	SoftDelete(ctx context.Context, tenantID, id uuid.UUID) (Asset, error)
	// Restore brings it back. Unlike documents there is nothing to collide with,
	// because assets have no unique field.
	Restore(ctx context.Context, tenantID, id uuid.UUID) (Asset, error)
}
