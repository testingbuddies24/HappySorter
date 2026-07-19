package nfo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/testingbuddies24/HappySorter/internal/scraper"
)

func TestWrite(t *testing.T) {
	m := &scraper.Metadata{
		Code:        "MIDA-678",
		Title:       "Example Title",
		Year:        2026,
		ReleaseDate: "2026-07-03",
		Studio:      "Moodyz",
		Director:    "Someone",
		Runtime:     125,
		Actresses:   []string{"篠真有"},
		Genres:      []string{"潮吹き", "中出し"},
	}
	path := filepath.Join(t.TempDir(), "out.nfo")
	art := Artwork{Poster: "MIDA-678 (2026)-poster.jpg", Fanart: "MIDA-678 (2026)-fanart.jpg"}

	if err := Write(path, m, art); err != nil {
		t.Fatalf("Write: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading nfo: %v", err)
	}
	got := string(data)

	// Fields the enriched schema must carry.
	wants := []string{
		"<title>[MIDA-678]Example Title</title>",
		"<num>MIDA-678</num>",
		"<release>2026-07-03</release>",
		"<maker>Moodyz</maker>",
		"<tag>中出し</tag>",
		"<genre>中出し</genre>",
		"<poster>MIDA-678 (2026)-poster.jpg</poster>",
		"<fanart>MIDA-678 (2026)-fanart.jpg</fanart>",
		`<uniqueid type="jav" default="true">MIDA-678</uniqueid>`,
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("nfo missing %q\n---\n%s", w, got)
		}
	}
}
