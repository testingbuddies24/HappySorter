// Package missav implements the FC2-only adapter for missav.ws. Verified
// live against the site (2026-08-19): detail pages sit at a predictable URL
// (/en/fc2-ppv-1234567, lowercase, keeping the "ppv" segment), unknown codes
// return a real HTTP 404, and no login, age gate, or Cloudflare challenge is
// involved — pages are served from Cloudflare's cache. One quirk: a browser
// User-Agent is mandatory (the default Go UA gets a 403).
//
// FC2 pages carry no actress row at all (the row is omitted, not "unknown"),
// so Actresses stays empty and the manager can fill it from another source.
package missav

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/testingbuddies24/HappySorter/internal/scraper"
)

const baseURL = "https://missav.ws"

type Adapter struct {
	client *http.Client
}

func New(client *http.Client) *Adapter {
	return &Adapter{client: client}
}

func (a *Adapter) Name() string { return "missav" }

func (a *Adapter) Capabilities() scraper.Capabilities {
	return scraper.Capabilities{Kind: scraper.KindAggregator}
}

// Lookup fetches the missav detail page directly for an FC2 code. Non-FC2
// codes are not this adapter's job (missav uses slugs, not codes, for
// regular releases — unverified), so they get an immediate not-found and the
// manager falls through to the next source.
func (a *Adapter) Lookup(ctx context.Context, code string) (*scraper.Metadata, error) {
	if !strings.HasPrefix(strings.ToUpper(code), "FC2") {
		return nil, scraper.ErrNotFound
	}
	url := fmt.Sprintf("%s/en/%s", baseURL, strings.ToLower(code))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("missav: building request: %w", err)
	}
	// Cloudflare passive fingerprinting: a browser UA alone gets the "Just a
	// moment..." challenge (verified live 2026-08-19 — UA-only 403s, the
	// full navigiation header set gets straight through).
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	resp, err := a.do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("missav: fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, scraper.ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("missav: unexpected status %d for %s", resp.StatusCode, url)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("missav: parsing %s: %w", url, err)
	}

	return parseDetail(doc, code)
}

// do sends req, retrying exactly once on Cloudflare's intermittent "Just a
// moment..." challenge (403). Observed live: consecutive requests seconds
// apart flip between challenged and served, so one delayed retry converts
// most challenges without retry-storming the site.
func (a *Adapter) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusForbidden {
		return resp, nil
	}
	resp.Body.Close()

	select {
	case <-time.After(2 * time.Second):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	retry, err := http.NewRequestWithContext(ctx, http.MethodGet, req.URL.String(), nil)
	if err != nil {
		return nil, err
	}
	for k, vs := range req.Header {
		retry.Header[k] = vs
	}
	return a.client.Do(retry)
}

// parseDetail reads the details-tab info panel: each row is a
// "div.text-secondary" whose first <span> is the label ("Title:", "Code:",
// "Release date:", "Genre:", "Maker:") and whose value is either a
// ".font-medium" element or a list of tag anchors.
//
// The page's own Code row must match the requested code: unknown codes
// redirect (302) to a fallback video page with HTTP 200, so the status code
// cannot be trusted to signal not-found.
func parseDetail(doc *goquery.Document, code string) (*scraper.Metadata, error) {
	meta := &scraper.Metadata{}
	pageCode := ""

	doc.Find("div.text-secondary").EachWithBreak(func(_ int, row *goquery.Selection) bool {
		label := strings.TrimSpace(row.Find("span").First().Text())
		switch label {
		case "Code:":
			pageCode = strings.TrimSpace(row.Find(".font-medium").First().Text())
		case "Title:":
			meta.Title = strings.TrimSpace(row.Find(".font-medium").First().Text())
		case "Release date:":
			// Prefer the time[datetime] attribute: the displayed text and
			// og:video:release_date can sit a day off across time zones.
			text := ""
			if dt, ok := row.Find("time").First().Attr("datetime"); ok {
				text = dt
			} else {
				text = strings.TrimSpace(row.Find(".font-medium").First().Text())
			}
			if len(text) >= 10 && text[4] == '-' && text[7] == '-' {
				meta.ReleaseDate = text[:10]
				if year, err := strconv.Atoi(text[:4]); err == nil {
					meta.Year = year
				}
			}
		case "Genre:":
			row.Find("a").Each(func(_ int, link *goquery.Selection) {
				meta.Genres = append(meta.Genres, strings.TrimSpace(link.Text()))
			})
		case "Maker:":
			meta.Studio = strings.TrimSpace(row.Find("a").First().Text())
		}
		return true
	})

	if meta.Title == "" || !strings.EqualFold(pageCode, code) {
		return nil, scraper.ErrNotFound
	}

	if cover, ok := doc.Find(`meta[property="og:image"]`).First().Attr("content"); ok {
		meta.CoverURL = cover
		meta.FanartURL = cover
	}

	return meta, nil
}
