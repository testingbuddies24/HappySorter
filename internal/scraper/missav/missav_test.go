package missav

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"

	"github.com/testingbuddies24/HappySorter/internal/scraper"
)

// Fixture trimmed from a live missav.ws response for FC2-PPV-4956890
// (fetched 2026-08-19): the details-tab info panel and the og:image tag.
const detailHTML = `<!DOCTYPE html>
<html>
<head>
<meta property="og:image" content="https://fourhoi.com/fc2-ppv-4956890/cover-n.jpg">
</head>
<body>
<div class="space-y-2">
	<div class="text-secondary">
		<span>Release date:</span>
		<time datetime="2026-08-08T00:00:00+08:00" class="font-medium">2026-08-08</time>
	</div>
	<div class="text-secondary">
		<span>Code:</span>
		<span class="font-medium">FC2-PPV-4956890</span>
	</div>
	<div class="text-secondary">
		<span>Title:</span>
		<span class="font-medium">オホッ♡オホッ♡っと再降臨！伝説のオホ声女神！</span>
	</div>
	<div class="text-secondary">
		<span>Genre:</span>
		<a href="https://missav.ws/dm218/en/genres/Beautiful%20Breasts" class="text-nord13 font-medium">Beautiful Breasts</a>, <a href="https://missav.ws/en/genres/Creampie" class="text-nord13 font-medium">Creampie</a>
	</div>
	<div class="text-secondary">
		<span>Maker:</span>
		<a href="https://missav.ws/dm445059/en/makers/Fc2" class="text-nord13 font-medium">Fc2</a>
	</div>
</div>
</body>
</html>`

func TestParseDetail(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(detailHTML))
	if err != nil {
		t.Fatalf("parsing fixture: %v", err)
	}

	meta, err := parseDetail(doc, "FC2-PPV-4956890")
	if err != nil {
		t.Fatalf("parseDetail: %v", err)
	}

	if meta.Title != "オホッ♡オホッ♡っと再降臨！伝説のオホ声女神！" {
		t.Errorf("Title = %q", meta.Title)
	}
	if meta.ReleaseDate != "2026-08-08" {
		t.Errorf("ReleaseDate = %q, want 2026-08-08 (from time[datetime], not display text)", meta.ReleaseDate)
	}
	if meta.Year != 2026 {
		t.Errorf("Year = %d, want 2026", meta.Year)
	}
	if meta.Studio != "Fc2" {
		t.Errorf("Studio = %q", meta.Studio)
	}
	wantGenres := []string{"Beautiful Breasts", "Creampie"}
	if len(meta.Genres) != len(wantGenres) {
		t.Fatalf("Genres = %v, want %v", meta.Genres, wantGenres)
	}
	for i, g := range wantGenres {
		if meta.Genres[i] != g {
			t.Errorf("Genres[%d] = %q, want %q", i, meta.Genres[i], g)
		}
	}
	if meta.CoverURL != "https://fourhoi.com/fc2-ppv-4956890/cover-n.jpg" {
		t.Errorf("CoverURL = %q", meta.CoverURL)
	}
	if meta.FanartURL != meta.CoverURL {
		t.Errorf("FanartURL = %q, want same as CoverURL", meta.FanartURL)
	}
}

func TestParseDetailNotAVideoPage(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<html><body><h1>MissAV</h1></body></html>`))
	if err != nil {
		t.Fatalf("parsing fixture: %v", err)
	}
	if _, err := parseDetail(doc, "FC2-PPV-4956890"); err != scraper.ErrNotFound {
		t.Errorf("parseDetail error = %v, want ErrNotFound", err)
	}
}

// Unknown codes 302 to a fallback video page (e.g. /en/fc2-ppv-1) with a
// normal 200, so the page's Code row — not the status — must decide.
func TestParseDetailFallbackPageForUnknownCode(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(detailHTML))
	if err != nil {
		t.Fatalf("parsing fixture: %v", err)
	}
	if _, err := parseDetail(doc, "FC2-PPV-000001"); err != scraper.ErrNotFound {
		t.Errorf("parseDetail error = %v, want ErrNotFound when page code doesn't match", err)
	}
}
