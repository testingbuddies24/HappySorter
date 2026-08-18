package javdb

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"

	"github.com/testingbuddies24/HappySorter/internal/scraper"
)

// Fixture trimmed from a live javdb.com FC2 search response (fetched
// 2026-08-19): the exact-match card first, then the near-miss neighbour a
// fuzzy search returns alongside it.
const searchHTML = `<!DOCTYPE html>
<html><body>
<div class="movie-list">
	<div class="item">
		<a href="/v/4DEYaG" class="box" title="【初撮り】上京したてで都会を知らない無垢な専門学校生。">
			<div class="cover ">
				<img loading="lazy" src="https://c0.jdbstatic.com/covers/4d/4DEYaG.jpg">
			</div>
			<div class="video-title">
				<strong>FC2-4956890</strong>
				【初撮り】上京したてで都会を知らない無垢な専門学校生。
			</div>
		</a>
	</div>
	<div class="item">
		<a href="/v/AbCdEf" class="box" title="近い番号の別の作品。">
			<div class="cover ">
				<img loading="lazy" src="https://c0.jdbstatic.com/covers/ab/AbCdEf.jpg">
			</div>
			<div class="video-title">
				<strong>FC2-4958891</strong>
				近い番号の別の作品。
			</div>
		</a>
	</div>
</div>
</body></html>`

func TestFC2Query(t *testing.T) {
	cases := []struct{ in, want string }{
		{"FC2-PPV-4956890", "FC2-4956890"}, // the raw extracted form must lose PPV
		{"fc2-ppv-4956890", "FC2-4956890"}, // lowercase input normalises too
		{"FC2-4956890", "FC2-4956890"},     // already-normalised stays put
	}
	for _, c := range cases {
		if got := fc2Query(c.in); got != c.want {
			t.Errorf("fc2Query(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseCard(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(searchHTML))
	if err != nil {
		t.Fatalf("parsing fixture: %v", err)
	}

	meta, err := parseCard(doc, fc2Query("FC2-PPV-4956890"))
	if err != nil {
		t.Fatalf("parseCard: %v", err)
	}

	if meta.Title != "【初撮り】上京したてで都会を知らない無垢な専門学校生。" {
		t.Errorf("Title = %q", meta.Title)
	}
	if meta.CoverURL != "https://c0.jdbstatic.com/covers/4d/4DEYaG.jpg" {
		t.Errorf("CoverURL = %q", meta.CoverURL)
	}
	if meta.FanartURL != meta.CoverURL {
		t.Errorf("FanartURL = %q, want same as CoverURL", meta.FanartURL)
	}
}

func TestParseCardOnlyFuzzyNeighbours(t *testing.T) {
	// The code isn't indexed: the search returns only near-miss cards, none
	// of which may be mistaken for a match.
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(searchHTML))
	if err != nil {
		t.Fatalf("parsing fixture: %v", err)
	}
	if _, err := parseCard(doc, "FC2-4956891"); err != scraper.ErrNotFound {
		t.Errorf("parseCard error = %v, want ErrNotFound when only neighbours return", err)
	}
}
