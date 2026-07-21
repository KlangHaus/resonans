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
}

// Store is the tenant-scoped persistence contract for asset metadata.
type Store interface {
	Create(ctx context.Context, tenantID uuid.UUID, a Asset) (Asset, error)
	Get(ctx context.Context, tenantID, id uuid.UUID) (Asset, error)
	List(ctx context.Context, tenantID uuid.UUID) ([]Asset, error)
}
