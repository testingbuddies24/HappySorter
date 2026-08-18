// Package javmenu implements the FC2-only adapter for javmenu.com. Verified
// live against the site (2026-08-19): no cookies, no age gate, no Cloudflare
// challenge. Two site-specific quirks handled here: the URL must drop the
// "PPV" segment (/en/FC2-1234567 — the FC2-PPV form soft-fails to a generic
// "You May Like" page), and unknown codes also return that soft-200 page
// rather than a real 404, so not-found is detected by the absence of the
// "Code:" row that only real video pages carry.
//
// FC2 pages list no actresses ("No actress info"), so Actresses stays empty
// and the manager can fill it from another source.
package javmenu

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/testingbuddies24/HappySorter/internal/scraper"
)

const baseURL = "https://javmenu.com"

type Adapter struct {
	client *http.Client
}

func New(client *http.Client) *Adapter {
	return &Adapter{client: client}
}

func (a *Adapter) Name() string { return "javmenu" }

func (a *Adapter) Capabilities() scraper.Capabilities {
	return scraper.Capabilities{Kind: scraper.KindAggregator}
}

// Lookup fetches the javmenu detail page directly for an FC2 code. Non-FC2
// codes are not this adapter's job (regular codes work on the site but were
// never verified against this parser), so they get an immediate not-found
// and the manager falls through to the next source.
func (a *Adapter) Lookup(ctx context.Context, code string) (*scraper.Metadata, error) {
	if !strings.HasPrefix(strings.ToUpper(code), "FC2") {
		return nil, scraper.ErrNotFound
	}
	// FC2-PPV-1234567 -> FC2-1234567: javmenu's URLs omit the PPV segment.
	code = strings.ToUpper(strings.Replace(code, "FC2-PPV-", "FC2-", 1))
	url := fmt.Sprintf("%s/en/%s", baseURL, code)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("javmenu: building request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("javmenu: fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("javmenu: unexpected status %d for %s", resp.StatusCode, url)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("javmenu: parsing %s: %w", url, err)
	}

	return parseDetail(doc, code)
}

// parseDetail reads the Video Info card (div.card-body). The h1 leads with
// the code and trails with a "Watch for free" call-to-action — both are
// stripped to leave the bare title. The page's own Code row must match the
// requested code so a generic fallback page can never masquerade as a hit.
func parseDetail(doc *goquery.Document, code string) (*scraper.Metadata, error) {
	// Only real video pages have the "Code:" row; soft-404 ("You May Like")
	// pages don't.
	codeRow := doc.Find("div.code")
	if codeRow.Length() == 0 {
		return nil, scraper.ErrNotFound
	}
	pageCode := strings.TrimSpace(codeRow.Text())
	pageCode = strings.Join(strings.Fields(strings.TrimPrefix(pageCode, "Code:")), "")
	if !strings.EqualFold(pageCode, code) {
		return nil, scraper.ErrNotFound
	}

	meta := &scraper.Metadata{}

	title := strings.TrimSpace(doc.Find("h1.display-5 strong").First().Text())
	title = strings.TrimSpace(strings.TrimPrefix(title, code))
	title = strings.TrimSpace(strings.TrimSuffix(title, "Watch for free"))
	meta.Title = title

	if meta.Title == "" {
		return nil, scraper.ErrNotFound
	}

	doc.Find(".card-body div.d-flex").Each(func(_ int, row *goquery.Selection) {
		label := strings.TrimSpace(row.Find("span").First().Text())
		switch label {
		case "Published At:":
			text := strings.TrimSpace(row.Find("span").Eq(1).Text())
			meta.ReleaseDate = text
			if len(text) >= 4 {
				if year, err := strconv.Atoi(text[:4]); err == nil {
					meta.Year = year
				}
			}
		}
	})

	if cover, ok := doc.Find(`meta[property="og:image"]`).First().Attr("content"); ok {
		meta.CoverURL = cover
		meta.FanartURL = cover
	}

	return meta, nil
}
