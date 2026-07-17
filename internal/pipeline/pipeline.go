// Package pipeline implements the per-file state machine described in
// docs/ARCHITECTURE.md § 2.2: filter -> extract code -> (Milestone 2:
// scrape -> organise).
package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/testingbuddies24/HappySorter/internal/config"
	"github.com/testingbuddies24/HappySorter/internal/fsutil"
	"github.com/testingbuddies24/HappySorter/internal/organiser"
	"github.com/testingbuddies24/HappySorter/internal/scraper"
	"github.com/testingbuddies24/HappySorter/internal/store"
)

type Pipeline struct {
	cfgStore     *config.Store
	store        *store.FileStore
	metaStore    *store.MetadataStore
	managerStore *scraper.ManagerStore
	organiser    *organiser.Organiser
	logger       *slog.Logger
}

func New(cfgStore *config.Store, st *store.FileStore, metaStore *store.MetadataStore, managerStore *scraper.ManagerStore, org *organiser.Organiser, logger *slog.Logger) *Pipeline {
	return &Pipeline{cfgStore: cfgStore, store: st, metaStore: metaStore, managerStore: managerStore, organiser: org, logger: logger}
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
			p.process(ctx, path)
		}
	}
}

// Retry re-runs the pipeline against a file sitting in a review folder —
// e.g. after the user has manually renamed it to inject a valid code. It
// treats the file's current on-disk path as a fresh path to process, so it
// is not skipped by the Seen() de-dup check that guards the original path.
func (p *Pipeline) Retry(ctx context.Context, path string) {
	p.process(ctx, path)
}

func (p *Pipeline) process(ctx context.Context, path string) {
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

	cfg := p.cfgStore.Get()

	if res := Filter(path, info.Size()); !res.Accepted {
		p.route(path, cfg.Paths.ReviewFilter, store.StateReviewFilter, "", res.Reason)
		return
	}

	code, ok := ExtractCode(path)
	if !ok {
		p.route(path, cfg.Paths.ReviewUnmatched, store.StateReviewUnmatched, "", "no JAV code found in filename")
		return
	}

	manager := p.managerStore.Get()
	if manager.Empty() {
		p.logger.Info("code extracted, queued for scrape", "path", path, "code", code)
		if err := p.store.Record(path, path, store.StateScrape, code, ""); err != nil {
			p.logger.Error("recording extracted file", "path", path, "error", err)
		}
		return
	}

	meta, err := p.lookupMetadata(ctx, manager, code)
	if err != nil {
		p.route(path, cfg.Paths.ReviewUnmatched, store.StateFailed, code, err.Error())
		return
	}

	dest, err := p.organiser.Organise(ctx, meta, path)
	if err != nil {
		var dupErr *organiser.DuplicateError
		if errors.As(err, &dupErr) {
			p.logger.Warn("duplicate file, routing for manual review", "path", path, "code", code, "existing", dupErr.ExistingPath)
			p.route(path, cfg.Paths.ReviewDuplicate, store.StateReviewDuplicate, code, err.Error())
			return
		}
		p.logger.Error("organising file", "path", path, "code", code, "error", err)
		p.route(path, cfg.Paths.ReviewUnmatched, store.StateFailed, code, "organise failed: "+err.Error())
		return
	}

	if err := p.metaStore.Put(meta); err != nil {
		p.logger.Error("caching metadata", "code", code, "error", err)
	}
	if err := p.store.Record(path, dest, store.StateDone, code, ""); err != nil {
		p.logger.Error("recording organised file", "path", path, "error", err)
	}
	p.logger.Info("organised", "path", path, "dest", dest, "code", code)
}

// lookupMetadata returns cached metadata for code if present (skipping a
// re-scrape for multi-disc releases), otherwise tries the scrape manager.
func (p *Pipeline) lookupMetadata(ctx context.Context, manager *scraper.Manager, code string) (*scraper.Metadata, error) {
	if cached, found, err := p.metaStore.Get(code); err != nil {
		p.logger.Error("reading metadata cache", "code", code, "error", err)
	} else if found {
		p.logger.Info("metadata cache hit, skipping scrape", "code", code)
		return cached, nil
	}

	meta, err := manager.Lookup(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("scrape failed: %w", err)
	}
	return meta, nil
}

// route moves a rejected file into a review folder and records its outcome.
func (p *Pipeline) route(path, reviewDir string, state store.FileState, code, reason string) {
	dest := fsutil.UniquePath(filepath.Join(reviewDir, filepath.Base(path)))
	if err := fsutil.MoveFile(path, dest); err != nil {
		p.logger.Error("moving file to review folder", "path", path, "dest", dest, "error", err)
		return
	}
	p.logger.Info("routed to review", "path", path, "dest", dest, "reason", reason)
	if err := p.store.Record(path, dest, state, code, reason); err != nil {
		p.logger.Error("recording routed file", "path", path, "error", err)
	}
}
