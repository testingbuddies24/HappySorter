package scraper

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

type fakeAdapter struct {
	name  string
	meta  *Metadata
	err   error
	calls int
}

func (a *fakeAdapter) Name() string               { return a.name }
func (a *fakeAdapter) Capabilities() Capabilities { return Capabilities{} }
func (a *fakeAdapter) Lookup(_ context.Context, _ string) (*Metadata, error) {
	a.calls++
	if a.err != nil {
		return nil, a.err
	}
	// Return a copy so the manager mutating meta.Source doesn't corrupt the
	// fixture between test cases.
	m := *a.meta
	return &m, nil
}

func TestManagerLookupStopsEarlyWhenFirstSourceIsComplete(t *testing.T) {
	first := &fakeAdapter{name: "s1", meta: &Metadata{
		Title: "Title", CoverURL: "cover.jpg", Plot: "plot", Director: "dir",
		Genres: []string{"g"}, Actresses: []string{"a"},
	}}
	second := &fakeAdapter{name: "javdb", meta: &Metadata{Title: "Title", CoverURL: "cover.jpg"}}

	m := NewManager(slog.Default(), first, second)
	got, err := m.Lookup(context.Background(), "CODE-001")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if second.calls != 0 {
		t.Errorf("expected second adapter not to be called when first is complete, got %d calls", second.calls)
	}
	if got.Source != "s1" {
		t.Errorf("Source = %q, want %q", got.Source, "s1")
	}
}

func TestManagerLookupMergesGapsFromFallbackSources(t *testing.T) {
	// Simulates javbus (no plot, has label) winning first, javdb filling
	// series/rating, s1-like plot never available from either.
	first := &fakeAdapter{name: "javbus", meta: &Metadata{
		Title: "Title", CoverURL: "cover.jpg", Director: "dir", Studio: "studio",
		Genres: []string{"g"}, Actresses: []string{"a"}, Label: "S1 NO.1 STYLE",
	}}
	second := &fakeAdapter{name: "javdb", meta: &Metadata{
		Title: "Title", CoverURL: "cover2.jpg", Series: "Some Series", Rating: 8.86,
	}}

	m := NewManager(slog.Default(), first, second)
	got, err := m.Lookup(context.Background(), "CODE-001")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if second.calls != 1 {
		t.Errorf("expected second adapter to be tried since first was missing Plot, got %d calls", second.calls)
	}
	if got.Label != "S1 NO.1 STYLE" {
		t.Errorf("Label = %q, want it preserved from the primary source", got.Label)
	}
	if got.Series != "Some Series" {
		t.Errorf("Series = %q, want it merged in from the fallback source", got.Series)
	}
	if got.Rating != 8.86 {
		t.Errorf("Rating = %v, want it merged in from the fallback source", got.Rating)
	}
	if got.CoverURL != "cover.jpg" {
		t.Errorf("CoverURL = %q, want the primary source's cover preserved, not overwritten", got.CoverURL)
	}
	if got.Source != "javbus,javdb" {
		t.Errorf("Source = %q, want provenance of both contributing adapters", got.Source)
	}
}

func TestManagerLookupSkipsIncompleteAndFailingSources(t *testing.T) {
	empty := &fakeAdapter{name: "broken", meta: &Metadata{Title: ""}}
	notFound := &fakeAdapter{name: "missing", err: ErrNotFound}
	good := &fakeAdapter{name: "s1", meta: &Metadata{Title: "Title", CoverURL: "cover.jpg"}}

	m := NewManager(slog.Default(), empty, notFound, good)
	got, err := m.Lookup(context.Background(), "CODE-001")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got.Title != "Title" {
		t.Errorf("Title = %q, want the only complete source's title", got.Title)
	}
}

func TestManagerLookupAllSourcesFail(t *testing.T) {
	a := &fakeAdapter{name: "a", err: ErrNotFound}
	m := NewManager(slog.Default(), a)
	if _, err := m.Lookup(context.Background(), "CODE-001"); err == nil {
		t.Fatal("expected an error when every adapter fails")
	}
}

// funcAdapter serves scripted answers, for tests that need a source whose
// behaviour changes between calls.
type funcAdapter struct {
	name   string
	lookup func() (*Metadata, error)
}

func (a *funcAdapter) Name() string               { return a.name }
func (a *funcAdapter) Capabilities() Capabilities { return Capabilities{} }
func (a *funcAdapter) Lookup(context.Context, string) (*Metadata, error) {
	return a.lookup()
}

// A source answering not-found for everything used to produce zero log
// lines (how the v1.0.12 outage hid for three weeks). The manager must stay
// quiet for isolated misses but call the source out once the run gets long.
func TestManagerWarnsOnceAtConsecutiveNotFoundThreshold(t *testing.T) {
	missing := &fakeAdapter{name: "javbus", err: ErrNotFound}
	var buf bytes.Buffer
	m := NewManager(slog.New(slog.NewTextHandler(&buf, nil)), missing)

	for i := 1; i < consecutiveNotFoundWarn; i++ {
		if _, err := m.Lookup(context.Background(), "CODE-001"); err == nil {
			t.Fatalf("Lookup %d: expected all-sources-failed error", i)
		}
	}
	if n := strings.Count(buf.String(), "not-found for every recent lookup"); n != 0 {
		t.Errorf("%d warnings before reaching %d consecutive not-founds:\n%s", n, consecutiveNotFoundWarn, buf.String())
	}

	if _, err := m.Lookup(context.Background(), "CODE-001"); err == nil {
		t.Fatal("expected all-sources-failed error")
	}
	if n := strings.Count(buf.String(), "not-found for every recent lookup"); n != 1 {
		t.Errorf("got %d warnings after %d consecutive not-founds, want exactly 1:\n%s", n, consecutiveNotFoundWarn, buf.String())
	}
}

func TestManagerNotFoundStreakResetsOnSuccess(t *testing.T) {
	// 4 misses, one success, then 5 more misses: exactly one warning, from
	// the second streak — the success must clear the first.
	script := []error{ErrNotFound, ErrNotFound, ErrNotFound, ErrNotFound, nil,
		ErrNotFound, ErrNotFound, ErrNotFound, ErrNotFound, ErrNotFound}
	i := 0
	flip := &funcAdapter{name: "javbus", lookup: func() (*Metadata, error) {
		defer func() { i++ }()
		if script[i] != nil {
			return nil, script[i]
		}
		return &Metadata{Title: "Title", CoverURL: "cover.jpg"}, nil
	}}

	var buf bytes.Buffer
	m := NewManager(slog.New(slog.NewTextHandler(&buf, nil)), flip)
	for range script {
		m.Lookup(context.Background(), "CODE-001")
	}

	if n := strings.Count(buf.String(), "not-found for every recent lookup"); n != 1 {
		t.Errorf("got %d warnings, want exactly 1 (streak reset by the intervening success):\n%s", n, buf.String())
	}
}

// ErrBlocked is a lockout, not a miss: it must hit the loud path (a warning
// on every occurrence), never the silent not-found path.
func TestManagerLogsBlockedSourcesLoudly(t *testing.T) {
	blocked := &fakeAdapter{name: "javbus", err: fmt.Errorf("javbus: %w", ErrBlocked)}
	var buf bytes.Buffer
	m := NewManager(slog.New(slog.NewTextHandler(&buf, nil)), blocked)

	m.Lookup(context.Background(), "CODE-001")
	if n := strings.Count(buf.String(), "source lookup failed"); n != 1 {
		t.Errorf(`ErrBlocked produced %d "source lookup failed" lines, want 1 (must not be swallowed like ErrNotFound):
%s`, n, buf.String())
	}
}

// Out-of-scope refusals are capability boundaries (javbus has no FC2; the
// FC2-only sources refuse everything else), decided with no request — they
// must stay silent and never feed the not-found streak, or an FC2-heavy
// ingest (or a mostly non-FC2 library with a FC2-only source enabled) would
// cry wolf about a healthy source.
func TestManagerOutOfScopeNeverCountsTowardStreak(t *testing.T) {
	fc2Only := &fakeAdapter{name: "missav", err: ErrOutOfScope}
	var buf bytes.Buffer
	m := NewManager(slog.New(slog.NewTextHandler(&buf, nil)), fc2Only)

	for i := 0; i < consecutiveNotFoundWarn+2; i++ {
		if _, err := m.Lookup(context.Background(), "CODE-001"); err == nil {
			t.Fatalf("Lookup %d: expected all-sources-failed error", i+1)
		}
	}
	if strings.Contains(buf.String(), "not-found for every recent lookup") {
		t.Errorf("out-of-scope refusals fed the not-found streak:\n%s", buf.String())
	}
	if strings.Contains(buf.String(), "source lookup failed") {
		t.Errorf("out-of-scope refusal was logged as a failure:\n%s", buf.String())
	}
}
