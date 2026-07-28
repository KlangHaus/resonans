package settings

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// MemoryStore is the in-memory Store for dev/tests.
type MemoryStore struct {
	mu   sync.RWMutex
	data map[uuid.UUID][]string
}

// NewMemoryStore returns an empty in-memory settings store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: map[uuid.UUID][]string{}}
}

var _ Store = (*MemoryStore)(nil)

func (m *MemoryStore) EnabledTypes(_ context.Context, tenantID uuid.UUID) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]string(nil), m.data[tenantID]...), nil
}

func (m *MemoryStore) SetEnabledTypes(_ context.Context, tenantID uuid.UUID, types []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[tenantID] = append([]string(nil), types...)
	return nil
}
