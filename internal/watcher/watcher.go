// Package watcher detects new files under a watched root
// (docs/ARCHITECTURE.md § 2.1).
package watcher

import (
	"context"
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
// filesystems (NFS/SMB) that don't reliably deliver inotify events.
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

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		w.logger.Warn("fsnotify unavailable, falling back to polling only", "error", err)
		w.pollLoop(ctx)
		return
	}
	defer fsw.Close()

	if err := w.addRecursive(fsw); err != nil {
		w.logger.Warn("could not set up fsnotify on watch root, falling back to polling only", "root", w.root, "error", err)
		w.pollLoop(ctx)
		return
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-fsw.Events:
			if !ok {
				return
			}
			if ev.Has(fsnotify.Create) || ev.Has(fsnotify.Write) {
				w.emit(ev.Name)
			}
		case err, ok := <-fsw.Errors:
			if !ok {
				return
			}
			w.logger.Error("fsnotify error", "error", err)
		case <-ticker.C:
			w.scan() // safety net for NFS/SMB even when fsnotify is active
		case <-w.rescanCh:
			w.scan()
		}
	}
}

func (w *Watcher) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.scan()
		case <-w.rescanCh:
			w.scan()
		}
	}
}

func (w *Watcher) addRecursive(fsw *fsnotify.Watcher) error {
	return filepath.WalkDir(w.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return fsw.Add(path)
		}
		return nil
	})
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
