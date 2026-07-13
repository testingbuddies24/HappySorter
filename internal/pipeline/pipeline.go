// Package pipeline implements the per-file state machine described in
// docs/ARCHITECTURE.md § 2.2: filter -> extract code -> (Milestone 2:
// scrape -> organise).
package pipeline

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/testingbuddies24/HappySorter/internal/config"
	"github.com/testingbuddies24/HappySorter/internal/store"
)

type Pipeline struct {
	cfg    *config.Config
	store  *store.FileStore
	logger *slog.Logger
}

func New(cfg *config.Config, st *store.FileStore, logger *slog.Logger) *Pipeline {
	return &Pipeline{cfg: cfg, store: st, logger: logger}
}

// Run consumes file paths from events until ctx is cancelled or events closes.
func (p *Pipeline) Run(ctx context.Context, events <-chan string) {
	for {
		select {
		case <-ctx.Done():
			return
		case path, ok := <-events:
			if !ok {
				return
			}
			p.process(path)
		}
	}
}

func (p *Pipeline) process(path string) {
	seen, err := p.store.Seen(path)
	if err != nil {
		p.logger.Error("checking file store", "path", path, "error", err)
		return
	}
	if seen {
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		// Common and harmless: the path was already moved/deleted by a
		// previous event for the same file, or vanished before we got to it.
		p.logger.Warn("stat failed, skipping", "path", path, "error", err)
		return
	}
	if info.IsDir() {
		return
	}

	if res := Filter(path, info.Size()); !res.Accepted {
		p.route(path, p.cfg.Paths.ReviewFilter, store.StateReviewFilter, "", res.Reason)
		return
	}

	code, ok := ExtractCode(path)
	if !ok {
		p.route(path, p.cfg.Paths.ReviewUnmatched, store.StateReviewUnmatched, "", "no JAV code found in filename")
		return
	}

	p.logger.Info("code extracted, queued for scrape", "path", path, "code", code)
	if err := p.store.Record(path, path, store.StateScrape, code, ""); err != nil {
		p.logger.Error("recording extracted file", "path", path, "error", err)
	}
}

// route moves a rejected file into a review folder and records its outcome.
func (p *Pipeline) route(path, reviewDir string, state store.FileState, code, reason string) {
	dest := uniquePath(filepath.Join(reviewDir, filepath.Base(path)))
	if err := moveFile(path, dest); err != nil {
		p.logger.Error("moving file to review folder", "path", path, "dest", dest, "error", err)
		return
	}
	p.logger.Info("routed to review", "path", path, "dest", dest, "reason", reason)
	if err := p.store.Record(path, dest, state, code, reason); err != nil {
		p.logger.Error("recording routed file", "path", path, "error", err)
	}
}
