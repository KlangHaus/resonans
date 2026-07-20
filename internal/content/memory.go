package content

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryStore is an in-memory Store for dev/test only. It is NOT persistent —
// content is lost on restart — so app.Run refuses to use it in production
// (mirrors the permissions in-memory-audit invariant).
type MemoryStore struct {
	mu      sync.RWMutex
	byID    map[uuid.UUID]Entry
	now     func() time.Time
	newUUID func() uuid.UUID
}

// NewMemoryStore constructs an empty in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		byID:    make(map[uuid.UUID]Entry),
		now:     time.Now,
		newUUID: uuid.New,
	}
}

var _ Store = (*MemoryStore)(nil)

func (s *MemoryStore) List(_ context.Context, f ListFilter) ([]Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Entry, 0, len(s.byID))
	for _, e := range s.byID {
		if f.Site != "" && e.Site != f.Site {
			continue
		}
		if f.Type != "" && e.Type != f.Type {
			continue
		}
		if f.Status != "" && e.Status != f.Status {
			continue
		}
		out = append(out, cloneEntry(e))
	}
	// Newest first, stable by ID for equal timestamps (deterministic tests).
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID.String() < out[j].ID.String()
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (s *MemoryStore) Get(_ context.Context, id uuid.UUID) (Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.byID[id]
	if !ok {
		return Entry{}, ErrNotFound
	}
	return cloneEntry(e), nil
}

func (s *MemoryStore) GetPublished(_ context.Context, site, slug string) (Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, e := range s.byID {
		if e.Site == site && e.Slug == slug && e.Status == StatusPublished {
			return cloneEntry(e), nil
		}
	}
	return Entry{}, ErrNotFound
}

func (s *MemoryStore) Create(_ context.Context, in CreateInput) (Entry, error) {
	if err := in.Validate(); err != nil {
		return Entry{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, e := range s.byID {
		if e.Site == in.Site && e.Slug == in.Slug {
			return Entry{}, ErrSlugConflict
		}
	}

	now := s.now().UTC()
	e := Entry{
		ID:        s.newUUID(),
		Site:      in.Site,
		Type:      in.Type,
		Slug:      in.Slug,
		Title:     in.Title,
		Body:      append(json.RawMessage(nil), in.Body...),
		Status:    StatusDraft,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.byID[e.ID] = e
	return cloneEntry(e), nil
}

func (s *MemoryStore) Update(_ context.Context, id uuid.UUID, in UpdateInput) (Entry, error) {
	if err := in.Validate(); err != nil {
		return Entry{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.byID[id]
	if !ok {
		return Entry{}, ErrNotFound
	}
	if in.Type != nil {
		e.Type = *in.Type
	}
	if in.Title != nil {
		e.Title = *in.Title
	}
	if in.Body != nil {
		e.Body = append(json.RawMessage(nil), (*in.Body)...)
	}
	e.UpdatedAt = s.now().UTC()
	s.byID[id] = e
	return cloneEntry(e), nil
}

func (s *MemoryStore) Publish(_ context.Context, id uuid.UUID) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.byID[id]
	if !ok {
		return Entry{}, ErrNotFound
	}
	now := s.now().UTC()
	e.Status = StatusPublished
	e.PublishedAt = &now
	e.UpdatedAt = now
	s.byID[id] = e
	return cloneEntry(e), nil
}

// cloneEntry deep-copies the mutable Body slice and PublishedAt pointer so
// callers can't mutate stored state through a returned Entry.
func cloneEntry(e Entry) Entry {
	c := e
	c.Body = append(json.RawMessage(nil), e.Body...)
	if e.PublishedAt != nil {
		t := *e.PublishedAt
		c.PublishedAt = &t
	}
	return c
}
