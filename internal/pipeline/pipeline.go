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
	"sync"
	"time"

	"github.com/testingbuddies24/HappySorter/internal/config"
	"github.com/testingbuddies24/HappySorter/internal/fsutil"
	"github.com/testingbuddies24/HappySorter/internal/organiser"
	"github.com/testingbuddies24/HappySorter/internal/scraper"
	"github.com/testingbuddies24/HappySorter/internal/store"
)

// settleWindow is how long a file must go unmodified before the pipeline
// will touch it. Files arriving over SMB/NFS are written incrementally for
// minutes; acting on one mid-copy would junk-route it on a partial size or
// organise a truncated video — and MoveFile's cross-mount copy+delete
// fallback would delete the source out from under the writer.
const settleWindow = 5 * time.Second

// unstableTTL is how long a path found mid-write is skipped outright before
// being re-checked, so a flood of fsnotify Write events during a long copy
// doesn't cost a settleWindow sleep each. Kept well under the watcher's 60s
// poll so a finished copy is picked up by the next poll after settling.
const unstableTTL = 20 * time.Second

type Pipeline struct {
	cfgStore     *config.Store
	store        *store.FileStore
	metaStore    *store.MetadataStore
	managerStore *scraper.ManagerStore
	organiser    *organiser.Organiser
	logger       *slog.Logger

	mu       sync.Mutex
	unstable map[string]time.Time // path -> when it was last seen mid-write
}

func New(cfgStore *config.Store, st *store.FileStore, metaStore *store.MetadataStore, managerStore *scraper.ManagerStore, org *organiser.Organiser, logger *slog.Logger) *Pipeline {
	return &Pipeline{cfgStore: cfgStore, store: st, metaStore: metaStore, managerStore: managerStore, organiser: org, logger: logger, unstable: make(map[string]time.Time)}
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

// DrainQueued reprocesses every file sitting in the scrape state — files
// whose code was extracted but no scraper source was enabled at the time
// (docs/ROADMAP.md M3 known-gaps: enabling a source later didn't
// previously drain this queue). Each record is cleared before reprocessing
// so Seen() doesn't skip it, mirroring the review-retry path.
func (p *Pipeline) DrainQueued(ctx context.Context) {
	files, err := p.store.ListByStates(store.StateScrape)
	if err != nil {
		p.logger.Error("listing queued files for drain", "error", err)
		return
	}
	for _, rec := range files {
		if err := p.store.Delete(rec.ID); err != nil {
			p.logger.Error("clearing queued record before drain", "id", rec.ID, "error", err)
			continue
		}
		p.Retry(ctx, rec.CurrentPath)
	}
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

	if p.recentlyUnstable(path) {
		return
	}
	settled, ok := waitForSettle(path, info, settleWindow)
	if !ok {
		p.markUnstable(path)
		p.logger.Info("file still being written, leaving it alone until it settles", "path", path)
		return
	}
	info = settled

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

// waitForSettle reports whether path has gone window without modification,
// sleeping (at most window) when the file is too fresh to tell yet. It
// returns the freshest FileInfo on success; ok=false means the file is
// still being written and the caller should skip it — the watcher's next
// poll re-emits the path, so nothing is lost by skipping.
func waitForSettle(path string, info os.FileInfo, window time.Duration) (os.FileInfo, bool) {
	age := time.Since(info.ModTime())
	if age >= window {
		return info, true
	}

	// Cap the sleep at window: a modest negative age just means the file
	// server's clock is slightly ahead of ours (common with SMB mounts).
	sleep := window - age
	if sleep > window {
		sleep = window
	}
	time.Sleep(sleep)

	latest, err := os.Stat(path)
	if err != nil {
		return nil, false // vanished mid-wait
	}
	if latest.Size() != info.Size() || !latest.ModTime().Equal(info.ModTime()) {
		return nil, false
	}
	return latest, true
}

// recentlyUnstable reports whether path was found mid-write within the last
// unstableTTL, letting the event-flood from a long copy short-circuit
// without paying waitForSettle's sleep every time.
func (p *Pipeline) recentlyUnstable(path string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	seen, ok := p.unstable[path]
	return ok && time.Since(seen) < unstableTTL
}

func (p *Pipeline) markUnstable(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, v := range p.unstable {
		if time.Since(v) >= unstableTTL {
			delete(p.unstable, k)
		}
	}
	p.unstable[path] = time.Now()
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
