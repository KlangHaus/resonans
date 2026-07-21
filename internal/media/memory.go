package media

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryStore is an in-memory asset Store for dev/test. It filters every
// operation on tenantID (simulating RLS).
type MemoryStore struct {
	mu      sync.RWMutex
	byID    map[uuid.UUID]Asset
	now     func() time.Time
	newUUID func() uuid.UUID
}

// NewMemoryStore constructs an empty in-memory asset store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{byID: map[uuid.UUID]Asset{}, now: time.Now, newUUID: uuid.New}
}

var _ Store = (*MemoryStore)(nil)

func (s *MemoryStore) Create(_ context.Context, tenantID uuid.UUID, a Asset) (Asset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a.ID = s.newUUID()
	a.TenantID = tenantID
	a.CreatedAt = s.now().UTC()
	if len(a.Hotspot) == 0 {
		a.Hotspot = json.RawMessage(`{}`)
	}
	s.byID[a.ID] = a
	return clone(a), nil
}

func (s *MemoryStore) Get(_ context.Context, tenantID, id uuid.UUID) (Asset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.byID[id]
	if !ok || a.TenantID != tenantID {
		return Asset{}, ErrNotFound
	}
	return clone(a), nil
}

func (s *MemoryStore) List(_ context.Context, tenantID uuid.UUID) ([]Asset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Asset, 0)
	for _, a := range s.byID {
		if a.TenantID == tenantID {
			out = append(out, clone(a))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID.String() < out[j].ID.String()
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func clone(a Asset) Asset {
	c := a
	c.Hotspot = append(json.RawMessage(nil), a.Hotspot...)
	if a.Width != nil {
		w := *a.Width
		c.Width = &w
	}
	if a.Height != nil {
		h := *a.Height
		c.Height = &h
	}
	return c
}
