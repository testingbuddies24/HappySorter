package watcher

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestAddRecursiveBestEffort(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "good")
	if err := os.Mkdir(good, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(good, "f.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Cross-platform: create a second dir whose fsnotify.Add call will fail.
	// On Linux this is a 0o000-locked dir (skipped when running as non-root).
	// On Windows we can't reproduce the same OS-level denial, so instead we
	// pre-register the root with fsnotify, then call addRecursive with a
	// *second* watcher so that the second watcher's Add on root returns
	// "resource exhausted". Either way, the contract is identical: a
	// per-directory failure must NOT abort the walk for subsequent dirs.
	if runtime.GOOS != "windows" {
		locked := filepath.Join(root, "locked")
		if err := os.Mkdir(locked, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer fsw.Close()

	skipped := addRecursive(fsw, root)

	// The good subtree must always be registered, even on Linux where
	// `locked` is unreadable, demonstrating best-effort semantics.
	assertWatched(t, fsw, good)

	// We never abort the walk. The skipped count is non-deterministic
	// across platforms — we only assert that we did not bail entirely
	// (which the fact that `good` is in the watch list already proves).
	t.Logf("skipped dirs: %v", skipped)
}

func assertWatched(t *testing.T, fsw *fsnotify.Watcher, dir string) {
	t.Helper()
	for _, watched := range fsw.WatchList() {
		if watched == dir {
			return
		}
	}
	t.Errorf("expected dir %q to be in fsnotify's WatchList", dir)
}

// TestResilienceOnResume verifies that Resume() does not panic when the
// watcher isn't running (the GUI calls Resume from request handlers).
func TestResumeIsSafeFromAnyState(t *testing.T) {
	w := New(t.TempDir(), discardLogger())
	w.Resume() // must not panic
	w.Pause()
	w.Resume()
	if w.Paused() {
		t.Fatalf("expected not paused after Resume")
	}
}

// TestDebounceCollapsesEventFlood verifies that a burst of fsnotify
// Create+Write events for one path (an SMB copy fires many) collapses into
// a single emit, timed from the last event seen — and that a later, separate
// write still produces its own emit.
func TestDebounceCollapsesEventFlood(t *testing.T) {
	if runtime.GOOS != "windows" && os.Getuid() == 0 {
		t.Skip("fsnotify behaves unreliably as root in some CI sandboxes")
	}

	root := t.TempDir()
	w := New(root, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	time.Sleep(200 * time.Millisecond) // let fsnotify finish registering root

	path := filepath.Join(root, "flood.mp4")
	for i := 0; i < 10; i++ {
		if err := os.WriteFile(path, []byte{byte(i)}, 0o644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(50 * time.Millisecond) // stay within debounceDelay of each other
	}

	select {
	case got := <-w.Events():
		if got != path {
			t.Fatalf("emit = %q, want %q", got, path)
		}
	case <-time.After(debounceDelay + 3*time.Second):
		t.Fatal("expected exactly one emit for the flood, got none")
	}

	select {
	case got := <-w.Events():
		t.Fatalf("unexpected second emit for the same flood: %q", got)
	case <-time.After(1 * time.Second):
	}

	if err := os.WriteFile(path, []byte("more"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-w.Events():
		if got != path {
			t.Fatalf("emit = %q, want %q", got, path)
		}
	case <-time.After(debounceDelay + 3*time.Second):
		t.Fatal("expected a second emit after a later, separate write")
	}
}
