package config

import "sync"

// Store guards a *Config behind a mutex so the GUI (internal/httpserver) can
// persist edits while the pipeline/organiser read paths and templates
// concurrently. Update swaps in an entirely new Config value (copy-on-write),
// so a *Config returned by Get before an Update remains a valid, unchanging
// snapshot — callers must not mutate it.
type Store struct {
	mu   sync.RWMutex
	path string
	cfg  *Config
}

// NewStore wraps an already-loaded Config for the given path.
func NewStore(path string, cfg *Config) *Store {
	return &Store{path: path, cfg: cfg}
}

// Get returns the current config snapshot. Treat it as read-only.
func (s *Store) Get() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Update applies mutate to a copy of the current config, persists it to
// config.yaml, and swaps it in on success. mutate must not retain next
// beyond the call.
func (s *Store) Update(mutate func(next *Config)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := *s.cfg
	mutate(&next)

	if err := save(s.path, &next); err != nil {
		return err
	}
	s.cfg = &next
	return nil
}
