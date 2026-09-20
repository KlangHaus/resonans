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
	mu       sync.RWMutex
	byID     map[uuid.UUID]Document
	versions []Version
	now      func() time.Time
	newUUID  func() uuid.UUID
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
		// The bin is a separate axis from status: a deleted document keeps the
		// status it had, so filtering on status alone would show it again.
		switch f.Deleted {
		case DeletedExclude:
			if d.DeletedAt != nil {
				continue
			}
		case DeletedOnly:
			if d.DeletedAt == nil {
				continue
			}
		case DeletedInclude:
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
		if d.TenantID == tenantID && d.Type == typ && d.Slug == slug && d.Status == StatusPublished && d.DeletedAt == nil {
			return cloneDoc(d), nil
		}
	}
	return Document{}, ErrNotFound
}

func (s *MemoryStore) GetBySlug(_ context.Context, tenantID uuid.UUID, typ, slug string) (Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.byID {
		if d.TenantID == tenantID && d.Type == typ && d.Slug == slug && d.DeletedAt == nil {
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

	// Only LIVE documents reserve a slug — the same rule the partial unique
	// index enforces in Postgres (migrations/0008). A page in the bin must not
	// block recreating it, or the bin becomes a trap instead of an undo.
	for _, d := range s.byID {
		if d.TenantID == tenantID && d.Dataset == DefaultDataset && d.Type == in.Type && d.Slug == in.Slug && d.DeletedAt == nil {
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
		Version:   1,
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
	if d.DeletedAt != nil {
		return Document{}, ErrDeleted
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
	d.Version++
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
	if d.DeletedAt != nil {
		return Document{}, ErrDeleted
	}
	now := s.now().UTC()
	d.Status = StatusPublished
	d.PublishedAt = &now
	d.Version++
	d.UpdatedAt = now
	s.byID[id] = d
	s.versions = append(s.versions, Version{
		ID: s.newUUID(), DocumentID: d.ID, Version: d.Version, Title: d.Title,
		Body: append(json.RawMessage(nil), d.Body...), Status: d.Status, CreatedAt: now,
	})
	return cloneDoc(d), nil
}

func (s *MemoryStore) ListVersions(_ context.Context, tenantID, docID uuid.UUID) ([]Version, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if d, ok := s.byID[docID]; !ok || d.TenantID != tenantID {
		return []Version{}, nil
	}
	out := make([]Version, 0)
	for i := len(s.versions) - 1; i >= 0; i-- {
		if s.versions[i].DocumentID == docID {
			v := s.versions[i]
			v.Body = append(json.RawMessage(nil), v.Body...)
			out = append(out, v)
		}
	}
	return out, nil
}

func (s *MemoryStore) Rollback(_ context.Context, tenantID, docID uuid.UUID, version int) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.byID[docID]
	if !ok || d.TenantID != tenantID {
		return Document{}, ErrNotFound
	}
	if d.DeletedAt != nil {
		return Document{}, ErrDeleted
	}
	var snap *Version
	for i := range s.versions {
		if s.versions[i].DocumentID == docID && s.versions[i].Version == version {
			snap = &s.versions[i]
			break
		}
	}
	if snap == nil {
		return Document{}, ErrNotFound
	}
	d.Title = snap.Title
	d.Body = append(json.RawMessage(nil), snap.Body...)
	d.Status = StatusDraft
	d.Version++
	d.UpdatedAt = s.now().UTC()
	s.byID[docID] = d
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

// SoftDelete puts a document in the bin. The row, its versions and its history
// stay; only `DeletedAt` changes. Version is NOT bumped: deleting is not an
// edit of the content, and bumping it would make the ETag change for a
// document nobody can read any more.
func (s *MemoryStore) SoftDelete(_ context.Context, tenantID, id uuid.UUID) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	d, ok := s.byID[id]
	if !ok || d.TenantID != tenantID {
		return Document{}, ErrNotFound
	}
	if d.DeletedAt != nil {
		return Document{}, ErrDeleted
	}
	now := s.now().UTC()
	d.DeletedAt = &now
	d.UpdatedAt = now
	s.byID[id] = d
	return cloneDoc(d), nil
}

// Restore takes a document back out of the bin — unless a live document has
// taken its slug in the meantime, in which case the LIVE one keeps it and the
// restore fails. Silently renaming either document would be worse: one of them
// would stop answering on the URL its readers use.
func (s *MemoryStore) Restore(_ context.Context, tenantID, id uuid.UUID) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	d, ok := s.byID[id]
	if !ok || d.TenantID != tenantID {
		return Document{}, ErrNotFound
	}
	if d.DeletedAt == nil {
		return Document{}, ErrNotDeleted
	}
	for _, other := range s.byID {
		if other.ID != d.ID && other.TenantID == tenantID && other.Dataset == d.Dataset &&
			other.Type == d.Type && other.Slug == d.Slug && other.DeletedAt == nil {
			return Document{}, ErrSlugConflict
		}
	}
	now := s.now().UTC()
	d.DeletedAt = nil
	d.UpdatedAt = now
	s.byID[id] = d
	return cloneDoc(d), nil
}
