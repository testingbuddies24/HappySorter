package scraper

import "sync/atomic"

// ManagerStore holds a swappable *Manager so the GUI can rebuild the
// adapter list (enable/disable, reorder) without restarting the process.
type ManagerStore struct {
	p atomic.Pointer[Manager]
}

// NewManagerStore wraps an initial Manager.
func NewManagerStore(m *Manager) *ManagerStore {
	s := &ManagerStore{}
	s.p.Store(m)
	return s
}

// Get returns the current Manager.
func (s *ManagerStore) Get() *Manager {
	return s.p.Load()
}

// Set swaps in a newly-built Manager.
func (s *ManagerStore) Set(m *Manager) {
	s.p.Store(m)
}
