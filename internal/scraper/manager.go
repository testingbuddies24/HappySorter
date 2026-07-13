package scraper

import (
	"context"
	"errors"
	"log/slog"
)

// Manager tries its adapters in order (studio-direct first, aggregators as
// fallback per docs/ARCHITECTURE.md § 4) and returns the first complete
// result.
type Manager struct {
	adapters []Adapter
	logger   *slog.Logger
}

// NewManager builds a Manager over adapters, tried in the order given —
// callers are expected to have already sorted them by config priority and
// filtered to enabled sources.
func NewManager(logger *slog.Logger, adapters ...Adapter) *Manager {
	return &Manager{adapters: adapters, logger: logger}
}

// Empty reports whether there are no enabled adapters at all — distinct
// from every adapter failing for a specific code.
func (m *Manager) Empty() bool {
	return len(m.adapters) == 0
}

// Lookup tries each adapter in order, returning the first result with a
// title and cover image. Errors from individual adapters are logged and
// treated as "try the next source" rather than aborting the whole lookup.
func (m *Manager) Lookup(ctx context.Context, code string) (*Metadata, error) {
	for _, a := range m.adapters {
		meta, err := a.Lookup(ctx, code)
		if err != nil {
			if !errors.Is(err, ErrNotFound) {
				m.logger.Warn("source lookup failed", "source", a.Name(), "code", code, "error", err)
			}
			continue
		}
		if meta.Title == "" || meta.CoverURL == "" {
			m.logger.Warn("source returned incomplete metadata, skipping", "source", a.Name(), "code", code)
			continue
		}
		meta.Code = code
		meta.Source = a.Name()
		return meta, nil
	}
	return nil, errUnableToLookup(code)
}

func errUnableToLookup(code string) error {
	return &lookupError{code: code}
}

type lookupError struct{ code string }

func (e *lookupError) Error() string {
	return "all sources failed for code " + e.code
}
