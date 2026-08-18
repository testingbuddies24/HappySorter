package javmenu

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"

	"github.com/testingbuddies24/HappySorter/internal/scraper"
)

// Fixture trimmed from a live javmenu.com response for FC2-4956891
// (fetched 2026-08-19): the h1 and the Video Info card.
const detailHTML = `<!DOCTYPE html>
<html>
<head>
<meta property="og:image" content="https://tukaka.space/video/m3u8/2026/08/12/c6982b2b/vod.jpg">
</head>
<body>
<div class="mb-3 px-1">
	<h1 class="display-5">
		<strong>
			FC2-4956891
			黑人禁断交合爆乳娇娃狂叫高潮不
			Watch for free		</strong>
	</h1>
</div>
<div class="left-wrapper mb-3">
	<div class="card rounded">
		<div class="card-body">
			<div class="d-flex align-items-center justify-content-between mb-2">
				<h5 class="card-title mb-0">Video Info</h5>
			</div>
			<div class="code d-flex mt-3">
				<span>Code:&nbsp;</span>
				<a href="https://javmenu.com/en/code/FC2">FC2</a><span>-4956891</span>
			</div>
			<div class="d-flex mt-1">
				<span>Published At:&nbsp;</span>
				<span>2026-08-13</span>
			</div>
			<div class="d-flex mt-1">
				<span>Updated At:&nbsp;</span>
				<span>2026-08-13</span>
			</div>
			<div class="d-flex mt-1">
				<span>AV idols:&nbsp;</span>
				<div>
					<span>No actress info</span>
				</div>
			</div>
		</div>
	</div>
</div>
</body>
</html>`

// Soft-404: unknown codes return HTTP 200 with a "You May Like" page that
// has no Video Info card — verified live for FC2-PPV-1733620.
const soft404HTML = `<!DOCTYPE html>
<html><head><title>You May Like | Complete Japanese AV Database</title></head>
<body><h1 class="display-5"><strong>You May Like</strong></h1></body></html>`

func TestParseDetail(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(detailHTML))
	if err != nil {
		t.Fatalf("parsing fixture: %v", err)
	}

	meta, err := parseDetail(doc, "FC2-4956891")
	if err != nil {
		t.Fatalf("parseDetail: %v", err)
	}

	if meta.Title != "黑人禁断交合爆乳娇娃狂叫高潮不" {
		t.Errorf("Title = %q, want code prefix and 'Watch for free' suffix stripped", meta.Title)
	}
	if meta.ReleaseDate != "2026-08-13" {
		t.Errorf("ReleaseDate = %q, want 2026-08-13", meta.ReleaseDate)
	}
	if meta.Year != 2026 {
		t.Errorf("Year = %d, want 2026", meta.Year)
	}
	if len(meta.Actresses) != 0 {
		t.Errorf("Actresses = %v, want empty (\"No actress info\" must not become one)", meta.Actresses)
	}
	if meta.CoverURL != "https://tukaka.space/video/m3u8/2026/08/12/c6982b2b/vod.jpg" {
		t.Errorf("CoverURL = %q", meta.CoverURL)
	}
}

func TestParseDetailSoft404(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(soft404HTML))
	if err != nil {
		t.Fatalf("parsing fixture: %v", err)
	}
	if _, err := parseDetail(doc, "FC2-1733620"); err != scraper.ErrNotFound {
		t.Errorf("parseDetail error = %v, want ErrNotFound on soft-404 page", err)
	}
}

func TestParseDetailPageForDifferentCode(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(detailHTML))
	if err != nil {
		t.Fatalf("parsing fixture: %v", err)
	}
	if _, err := parseDetail(doc, "FC2-000001"); err != scraper.ErrNotFound {
		t.Errorf("parseDetail error = %v, want ErrNotFound when page code doesn't match", err)
	}
}
