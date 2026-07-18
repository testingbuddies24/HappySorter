// Package watcher detects new files under a watched root
// (docs/ARCHITECTURE.md § 2.1).
package watcher

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

const pollInterval = 60 * time.Second

// Watcher emits paths of files found under root: an initial full scan on
// startup (catches anything dropped while offline), a recursive fsnotify
// watch for near-instant detection, and a periodic poll as a safety net for
// filesystems (NFS/SMB, Synology ACL-protected shares, etc.) that don't
// reliably deliver inotify events.
//
// The polling ticker ALWAYS runs regardless of fsnotify setup success —
// inotify is best-effort. fsnotify may partially fail (e.g. one ACL-locked
// dir under root), but a single denied dir no longer takes down the whole
// watcher: setup is best-effort per-directory and the poll catches what
// inotify misses.
//
// Emitting the same path more than once is expected and safe — the
// pipeline consuming Events() de-duplicates against the `files` table.
type Watcher struct {
	root     string
	logger   *slog.Logger
	events   chan string
	paused   atomic.Bool
	rescanCh chan struct{}
}

func New(root string, logger *slog.Logger) *Watcher {
	return &Watcher{
		root:     root,
		logger:   logger,
		events:   make(chan string, 256),
		rescanCh: make(chan struct{}, 1),
	}
}

func (w *Watcher) Events() <-chan string { return w.events }

// Pause stops new files from being emitted for processing. Detection
// (fsnotify/polling) keeps running underneath so nothing is missed; Resume
// picks back up with a full scan.
func (w *Watcher) Pause() { w.paused.Store(true) }

// Resume re-enables emission and immediately triggers a scan to catch
// anything dropped while paused.
func (w *Watcher) Resume() {
	w.paused.Store(false)
	w.Rescan()
}

// Paused reports whether the watcher is currently paused.
func (w *Watcher) Paused() bool { return w.paused.Load() }

// Rescan requests an out-of-band full scan of root, in addition to the
// normal fsnotify/poll-driven detection. Safe to call at any time; a scan
// already pending is not duplicated.
func (w *Watcher) Rescan() {
	select {
	case w.rescanCh <- struct{}{}:
	default:
	}
}

// Run blocks until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) {
	defer close(w.events)

	if _, err := os.Stat(w.root); err != nil {
		w.logger.Warn("watch folder not found yet; will pick it up once it exists", "root", w.root, "error", err)
	}

	w.scan()

	// inotify setup is best-effort: a single ACL-protected or otherwise
	// inaccessible directory under root shouldn't take down the whole
	// watcher. The poll ticker below always runs.
	fsw, fswErr := fsnotify.NewWatcher()
	if fswErr != nil {
		w.logger.Warn("fsnotify unavailable, falling back to polling only", "error", fswErr)
		fsw = nil
	}
	if fsw != nil {
		defer fsw.Close()
		if skipped := addRecursive(fsw, w.root); len(skipped) > 0 {
			w.logger.Warn("some directories skipped by fsnotify (polling still active)", "count", len(skipped), "first", skipped[0])
		}
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-fswOrNil(fsw):
			if !ok {
				fsw = nil
				continue
			}
			if ev.Has(fsnotify.Create) || ev.Has(fsnotify.Write) {
				w.emit(ev.Name)
			}
		case err, ok := <-fswOrNilErrors(fsw):
			if !ok {
				fsw = nil
				continue
			}
			w.logger.Error("fsnotify error", "error", err)
		case <-ticker.C:
			w.scan() // safety net for NFS/SMB even when fsnotify is active
		case <-w.rescanCh:
			w.scan()
		}
	}
}

// fswOrNil safely returns the fsnotify event channel, or a never-valid
// channel when inotify setup failed or was forcibly torn down mid-run. The
// receive on a nil channel blocks forever, which is exactly what the
// select-loop needs.
func fswOrNil(fsw *fsnotify.Watcher) <-chan fsnotify.Event {
	if fsw == nil {
		return nil
	}
	return fsw.Events
}

// fswOrNilErrors mirrors fswOrNil for the error channel.
func fswOrNilErrors(fsw *fsnotify.Watcher) <-chan error {
	if fsw == nil {
		return nil
	}
	return fsw.Errors
}

// addRecursive registers fsnotify on root and every directory beneath it.
// Per-directory errors (permission-denied, ACL issues on Synology shares,
// etc.) are collected and returned as a slice — the caller logs them but
// continues; the polling-driven scan in Run() still picks up new files.
func addRecursive(fsw *fsnotify.Watcher, root string) []string {
	var skipped []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("%s: %v", path, err))
			return nil //nolint:nilerr // best-effort per-dir; do not abort the walk
		}
		if !d.IsDir() {
			return nil
		}
		if err := fsw.Add(path); err != nil {
			skipped = append(skipped, fmt.Sprintf("%s: %v", path, err))
		}
		return nil
	})
	return skipped
}

func (w *Watcher) scan() {
	_ = filepath.WalkDir(w.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // best-effort scan; missing root is expected pre-mount
		}
		if !d.IsDir() {
			w.emit(path)
		}
		return nil
	})
}

func (w *Watcher) emit(path string) {
	if w.paused.Load() {
		return
	}
	select {
	case w.events <- path:
	default:
		w.logger.Warn("event channel full, dropping path (will be caught by next poll)", "path", path)
	}
}
