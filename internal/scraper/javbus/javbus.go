// Package javbus implements the aggregator adapter for javbus.com. Verified
// live against the site: the detail page sits at a predictable URL
// (hyphenated code, e.g. /SSIS-001). Since early October 2026 the site
// age/bot-gates requests that carry no Accept-Language header: they get a
// 302 to a /doc/driver-verify interstitial with an EMPTY body (until then
// the gate tucked the real page HTML into the 302 body, which is why this
// adapter parses whichever body comes back instead of following
// redirects). Sending any Accept-Language header — as every real browser
// does — passes the gate: found codes return 200 with the real page, and
// unknown codes a plain 404. No proxy or persisted cookie is needed.
package javbus

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/testingbuddies24/HappySorter/internal/scraper"
)

var baseURL = "https://www.javbus.com"

var runtimeRegex = regexp.MustCompile(`(\d+)\s*分`)

type Adapter struct {
	client *http.Client
}

// New wraps client in a shallow copy with redirect-following disabled, so
// that if the age/bot gate (or any future redirect quirk) does answer with
// a 302, the adapter parses whatever body comes back rather than chasing
// the interstitial.
func New(client *http.Client) *Adapter {
	noRedirect := *client
	noRedirect.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Adapter{client: &noRedirect}
}

func (a *Adapter) Name() string { return "javbus" }

func (a *Adapter) Capabilities() scraper.Capabilities {
	return scraper.Capabilities{Kind: scraper.KindAggregator}
}

// Lookup fetches the JavBus detail page directly for code.
//
// FC2 codes get an immediate not-found: fc2.javbus.com lost its DNS record
// (verified 2026-08-19: NODATA from javbus.com's own authoritative
// nameservers), and the main domain returns a real 404 page for every FC2
// URL form (/FC2-PPV-4956891, /FC2-4956891). JavBus no longer hosts FC2
// content — the missav/javmenu/javdb-card adapters own those codes now.
func (a *Adapter) Lookup(ctx context.Context, code string) (*scraper.Metadata, error) {
	if strings.HasPrefix(strings.ToUpper(code), "FC2") {
		return nil, scraper.ErrNotFound
	}
	url := fmt.Sprintf("%s/%s", baseURL, code)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("javbus: building request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")
	// Required since the October 2026 age/bot gate: requests without an
	// Accept-Language header get a 302 to the driver-verify interstitial
	// with an empty body instead of the page (see package doc).
	req.Header.Set("Accept-Language", "zh-TW,zh;q=0.9,en;q=0.8")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("javbus: fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	// Unknown codes now return a plain 404 (with the header set) instead of
	// the old 302 carrying a "404 Page Not Found!" body.
	if resp.StatusCode == http.StatusNotFound {
		return nil, scraper.ErrNotFound
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusFound {
		return nil, fmt.Errorf("javbus: unexpected status %d for %s", resp.StatusCode, url)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("javbus: parsing %s: %w", url, err)
	}

	info := doc.Find(".col-md-3.info")
	if info.Length() == 0 {
		return nil, scraper.ErrNotFound
	}

	title := strings.TrimSpace(doc.Find("h3").First().Text())
	title = strings.TrimSpace(strings.TrimPrefix(title, code))

	meta := &scraper.Metadata{Title: title}

	info.Find("p").Each(func(_ int, p *goquery.Selection) {
		label := strings.TrimSpace(p.Find(".header").First().Text())
		switch {
		case strings.HasPrefix(label, "發行日期"):
			text := strings.TrimSpace(strings.TrimPrefix(p.Text(), label))
			if len(text) >= 4 {
				meta.ReleaseDate = text
				if year, err := strconv.Atoi(text[:4]); err == nil {
					meta.Year = year
				}
			}
		case strings.HasPrefix(label, "長度"):
			if m := runtimeRegex.FindStringSubmatch(p.Text()); m != nil {
				meta.Runtime, _ = strconv.Atoi(m[1])
			}
		case strings.HasPrefix(label, "導演"):
			meta.Director = strings.TrimSpace(p.Find("a").First().Text())
		case strings.HasPrefix(label, "製作商"):
			meta.Studio = strings.TrimSpace(p.Find("a").First().Text())
		case strings.HasPrefix(label, "發行商"):
			meta.Label = strings.TrimSpace(p.Find("a").First().Text())
		}
	})

	// Scoped to ".genre label a": JavBus reuses the "genre" class on a second,
	// unrelated block of hover-card spans wrapping the actress links further
	// down the same info column, but only the real genre tags are wrapped in
	// a <label> (around the tag's own checkbox input) — the actress spans
	// aren't, so this excludes them without excluding real genres.
	info.Find(".genre label a").Each(func(_ int, link *goquery.Selection) {
		meta.Genres = append(meta.Genres, strings.TrimSpace(link.Text()))
	})
	info.Find(".star-name a").Each(func(_ int, link *goquery.Selection) {
		meta.Actresses = append(meta.Actresses, strings.TrimSpace(link.Text()))
	})

	if cover, ok := doc.Find("a.bigImage").First().Attr("href"); ok {
		meta.CoverURL = resolveURL(cover)
		meta.FanartURL = meta.CoverURL
	}

	return meta, nil
}

func resolveURL(href string) string {
	if strings.HasPrefix(href, "http") {
		return href
	}
	return baseURL + href
}
