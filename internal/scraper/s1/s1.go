// Package s1 implements the studio-direct adapter for s1s1s1.com
// (S1 NO.1 STYLE). Verified live against the site (docs/research/
// source-test-results.md): no Cloudflare, no age gate, and the product
// detail page sits at a predictable URL, so lookup skips the search step
// entirely. Two site-specific quirks handled here: the hyphen must be
// stripped from the code for S1's URLs, and unknown codes return HTTP 200
// with a generic page instead of a real 404 (detected by the absence of
// the title element).
package s1

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

const baseURL = "https://s1s1s1.com"

const studioName = "S1 NO.1 STYLE"

var (
	releaseDateRegex = regexp.MustCompile(`(\d{4})年(\d{1,2})月(\d{1,2})日`)
	runtimeRegex     = regexp.MustCompile(`(\d+)分`)
)

type Adapter struct {
	client *http.Client
}

func New(client *http.Client) *Adapter {
	return &Adapter{client: client}
}

func (a *Adapter) Name() string { return "s1" }

func (a *Adapter) Capabilities() scraper.Capabilities {
	return scraper.Capabilities{Kind: scraper.KindStudio}
}

// Lookup fetches the S1 product detail page directly for code.
func (a *Adapter) Lookup(ctx context.Context, code string) (*scraper.Metadata, error) {
	slug := strings.ReplaceAll(code, "-", "")
	url := fmt.Sprintf("%s/works/detail/%s", baseURL, slug)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("s1: building request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("s1: fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("s1: unexpected status %d for %s", resp.StatusCode, url)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("s1: parsing %s: %w", url, err)
	}

	title := strings.TrimSpace(doc.Find("h2.p-workPage__title").First().Text())
	if title == "" {
		return nil, scraper.ErrNotFound
	}

	meta := &scraper.Metadata{
		Title:  title,
		Plot:   strings.TrimSpace(doc.Find("p.p-workPage__text").First().Text()),
		Studio: studioName,
	}

	doc.Find(".p-workPage__table > .item").Each(func(_ int, row *goquery.Selection) {
		label := strings.TrimSpace(row.Find(".th").First().Text())
		td := row.Find(".td").First()
		switch label {
		case "女優":
			td.Find(".item a").Each(func(_ int, link *goquery.Selection) {
				meta.Actresses = append(meta.Actresses, strings.TrimSpace(link.Text()))
			})
		case "発売日":
			text := strings.TrimSpace(td.Find("a").First().Text())
			if m := releaseDateRegex.FindStringSubmatch(text); m != nil {
				year, _ := strconv.Atoi(m[1])
				month, _ := strconv.Atoi(m[2])
				day, _ := strconv.Atoi(m[3])
				meta.Year = year
				meta.ReleaseDate = fmt.Sprintf("%04d-%02d-%02d", year, month, day)
			}
		case "ジャンル":
			td.Find(".item a").Each(func(_ int, link *goquery.Selection) {
				meta.Genres = append(meta.Genres, strings.TrimSpace(link.Text()))
			})
		case "監督":
			meta.Director = strings.TrimSpace(td.Find("p").First().Text())
		case "収録時間":
			if m := runtimeRegex.FindStringSubmatch(td.Find("p").First().Text()); m != nil {
				meta.Runtime, _ = strconv.Atoi(m[1])
			}
		}
	})

	if cover, ok := doc.Find(".swiper-slide img").First().Attr("data-src"); ok {
		meta.CoverURL = cover
		// S1 doesn't publish a separate wide/backdrop image, so the box
		// cover is reused as fanart.
		meta.FanartURL = cover
	}

	return meta, nil
}
