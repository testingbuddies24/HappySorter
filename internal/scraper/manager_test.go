package scraper

import (
	"context"
	"log/slog"
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
