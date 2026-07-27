package scraper

import (
	"context"
	"errors"
	"log/slog"
)

// Manager tries its adapters in order (studio-direct first, aggregators as
// fallback per docs/ARCHITECTURE.md § 4), accepts the first result with a
// title and cover image, and fills any gaps that result leaves (e.g. no
// Plot, no Director) from subsequent sources rather than settling for
// whichever adapter happened to answer first.
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

// Lookup tries adapters in order. The first result with a title and cover
// image is accepted as the base; if it's missing any of the core enrichment
// fields (Plot, Director, Genres, Actresses), remaining adapters are tried
// too and merged in to fill those gaps — but only until the base is
// "complete" by that measure, so the common case (a studio-direct source
// answering everything) costs exactly one request, same as before.
func (m *Manager) Lookup(ctx context.Context, code string) (*Metadata, error) {
	var merged *Metadata

	for _, a := range m.adapters {
		if merged != nil && isComplete(merged) {
			break
		}

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
		meta.Source = a.Name()

		if merged == nil {
			merged = meta
			continue
		}
		if filled := mergeFields(merged, meta); filled {
			m.logger.Info("merged additional fields from fallback source", "source", a.Name(), "code", code)
		}
	}

	if merged == nil {
		return nil, errUnableToLookup(code)
	}
	merged.Code = code
	return merged, nil
}

// isComplete reports whether m already has the fields that most affect NFO
// quality. Series/Label/Rating are deliberately excluded: they're genuinely
// absent from most sources, and requiring them would force every lookup to
// exhaust all enabled adapters for no benefit.
func isComplete(m *Metadata) bool {
	return m.Plot != "" && m.Director != "" && len(m.Genres) > 0 && len(m.Actresses) > 0
}

// mergeFields copies any field src has that dst is missing. Reports whether
// it changed anything, so the caller can decide whether src contributed to
// the provenance trail.
func mergeFields(dst, src *Metadata) bool {
	filled := false
	fill := func() { filled = true }

	if dst.Plot == "" && src.Plot != "" {
		dst.Plot = src.Plot
		fill()
	}
	if dst.Director == "" && src.Director != "" {
		dst.Director = src.Director
		fill()
	}
	if dst.Studio == "" && src.Studio != "" {
		dst.Studio = src.Studio
		fill()
	}
	if dst.Year == 0 && src.Year != 0 {
		dst.Year = src.Year
		fill()
	}
	if dst.ReleaseDate == "" && src.ReleaseDate != "" {
		dst.ReleaseDate = src.ReleaseDate
		fill()
	}
	if dst.Runtime == 0 && src.Runtime != 0 {
		dst.Runtime = src.Runtime
		fill()
	}
	if len(dst.Genres) == 0 && len(src.Genres) > 0 {
		dst.Genres = src.Genres
		fill()
	}
	if len(dst.Actresses) == 0 && len(src.Actresses) > 0 {
		dst.Actresses = src.Actresses
		fill()
	}
	if dst.Series == "" && src.Series != "" {
		dst.Series = src.Series
		fill()
	}
	if dst.Label == "" && src.Label != "" {
		dst.Label = src.Label
		fill()
	}
	if dst.Rating == 0 && src.Rating != 0 {
		dst.Rating = src.Rating
		fill()
	}
	if dst.FanartURL == "" && src.FanartURL != "" {
		dst.FanartURL = src.FanartURL
		fill()
	}

	if filled {
		dst.Source = dst.Source + "," + src.Source
	}
	return filled
}

func errUnableToLookup(code string) error {
	return &lookupError{code: code}
}

type lookupError struct{ code string }

func (e *lookupError) Error() string {
	return "all sources failed for code " + e.code
}
