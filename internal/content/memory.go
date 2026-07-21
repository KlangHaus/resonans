package content

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryStore is an in-memory Store for dev/test only. It is NOT persistent, so
// app.Run refuses to use it in production. It simulates RLS by filtering every
// operation on tenantID.
type MemoryStore struct {
	mu      sync.RWMutex
	byID    map[uuid.UUID]Document
	now     func() time.Time
	newUUID func() uuid.UUID
}

// NewMemoryStore constructs an empty in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		byID:    make(map[uuid.UUID]Document),
		now:     time.Now,
		newUUID: uuid.New,
	}
}

var _ Store = (*MemoryStore)(nil)

func (s *MemoryStore) List(_ context.Context, tenantID uuid.UUID, f ListFilter) ([]Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Document, 0)
	for _, d := range s.byID {
		if d.TenantID != tenantID {
			continue
		}
		if f.Type != "" && d.Type != f.Type {
			continue
		}
		if f.Status != "" && d.Status != f.Status {
			continue
		}
		out = append(out, cloneDoc(d))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID.String() < out[j].ID.String()
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (s *MemoryStore) Get(_ context.Context, tenantID, id uuid.UUID) (Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.byID[id]
	if !ok || d.TenantID != tenantID {
		return Document{}, ErrNotFound
	}
	return cloneDoc(d), nil
}

func (s *MemoryStore) GetPublished(_ context.Context, tenantID uuid.UUID, typ, slug string) (Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.byID {
		if d.TenantID == tenantID && d.Type == typ && d.Slug == slug && d.Status == StatusPublished {
			return cloneDoc(d), nil
		}
	}
	return Document{}, ErrNotFound
}

func (s *MemoryStore) Create(_ context.Context, tenantID uuid.UUID, in CreateInput) (Document, error) {
	if err := in.Validate(); err != nil {
		return Document{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, d := range s.byID {
		if d.TenantID == tenantID && d.Dataset == DefaultDataset && d.Type == in.Type && d.Slug == in.Slug {
			return Document{}, ErrSlugConflict
		}
	}

	now := s.now().UTC()
	d := Document{
		ID:        s.newUUID(),
		TenantID:  tenantID,
		Dataset:   DefaultDataset,
		Type:      in.Type,
		Slug:      in.Slug,
		Title:     in.Title,
		Body:      append(json.RawMessage(nil), in.Body...),
		Status:    StatusDraft,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.byID[d.ID] = d
	return cloneDoc(d), nil
}

func (s *MemoryStore) Update(_ context.Context, tenantID, id uuid.UUID, in UpdateInput) (Document, error) {
	if err := in.Validate(); err != nil {
		return Document{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	d, ok := s.byID[id]
	if !ok || d.TenantID != tenantID {
		return Document{}, ErrNotFound
	}
	if in.Type != nil {
		d.Type = *in.Type
	}
	if in.Title != nil {
		d.Title = *in.Title
	}
	if in.Body != nil {
		d.Body = append(json.RawMessage(nil), (*in.Body)...)
	}
	d.UpdatedAt = s.now().UTC()
	s.byID[id] = d
	return cloneDoc(d), nil
}

func (s *MemoryStore) Publish(_ context.Context, tenantID, id uuid.UUID) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	d, ok := s.byID[id]
	if !ok || d.TenantID != tenantID {
		return Document{}, ErrNotFound
	}
	now := s.now().UTC()
	d.Status = StatusPublished
	d.PublishedAt = &now
	d.UpdatedAt = now
	s.byID[id] = d
	return cloneDoc(d), nil
}

// cloneDoc deep-copies the mutable Body slice and PublishedAt pointer so callers
// cannot mutate stored state through a returned Document.
func cloneDoc(d Document) Document {
	c := d
	c.Body = append(json.RawMessage(nil), d.Body...)
	if d.PublishedAt != nil {
		t := *d.PublishedAt
		c.PublishedAt = &t
	}
	return c
}
