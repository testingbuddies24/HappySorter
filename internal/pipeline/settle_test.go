package pipeline

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWaitForSettleOldFilePassesImmediately(t *testing.T) {
	path := writeTemp(t, "old.mp4", []byte("data"))
	past := time.Now().Add(-time.Minute)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}

	info := mustStat(t, path)
	start := time.Now()
	settled, ok := waitForSettle(path, info, 500*time.Millisecond)
	if !ok {
		t.Fatal("expected old file to be settled")
	}
	if settled.Size() != info.Size() {
		t.Fatalf("size mismatch: %d != %d", settled.Size(), info.Size())
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Errorf("expected no sleep for an old file, took %v", elapsed)
	}
}

func TestWaitForSettleFreshUnchangedFilePasses(t *testing.T) {
	path := writeTemp(t, "fresh.mp4", []byte("data"))
	info := mustStat(t, path)

	settled, ok := waitForSettle(path, info, 100*time.Millisecond)
	if !ok {
		t.Fatal("expected fresh-but-unchanged file to settle after the wait")
	}
	if settled.Size() != info.Size() {
		t.Fatalf("size mismatch: %d != %d", settled.Size(), info.Size())
	}
}

func TestWaitForSettleGrowingFileFails(t *testing.T) {
	path := writeTemp(t, "growing.mp4", []byte("data"))
	info := mustStat(t, path)

	// Simulate a copy in progress: the file grows after the initial stat.
	if err := os.WriteFile(path, []byte("data plus more bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, ok := waitForSettle(path, info, 100*time.Millisecond); ok {
		t.Fatal("expected a file that changed during the window to be reported unsettled")
	}
}

func TestWaitForSettleVanishedFileFails(t *testing.T) {
	path := writeTemp(t, "vanish.mp4", []byte("data"))
	info := mustStat(t, path)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	if _, ok := waitForSettle(path, info, 50*time.Millisecond); ok {
		t.Fatal("expected a vanished file to be reported unsettled")
	}
}

func TestUnstableDebounce(t *testing.T) {
	p := &Pipeline{unstable: make(map[string]time.Time)}

	if p.recentlyUnstable("/watch/a.mp4") {
		t.Fatal("fresh pipeline should have no unstable entries")
	}
	p.markUnstable("/watch/a.mp4")
	if !p.recentlyUnstable("/watch/a.mp4") {
		t.Fatal("path should be debounced right after being marked")
	}
	if p.recentlyUnstable("/watch/b.mp4") {
		t.Fatal("unrelated path must not be debounced")
	}

	// Expired entries are treated as not-unstable and pruned on next mark.
	p.unstable["/watch/a.mp4"] = time.Now().Add(-2 * unstableTTL)
	if p.recentlyUnstable("/watch/a.mp4") {
		t.Fatal("expired entry should no longer debounce")
	}
	p.markUnstable("/watch/c.mp4")
	if _, exists := p.unstable["/watch/a.mp4"]; exists {
		t.Fatal("expired entry should have been pruned by markUnstable")
	}
}

func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func mustStat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}
