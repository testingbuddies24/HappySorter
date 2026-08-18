// Package javdb implements the aggregator adapter for javdb.com. Verified
// live against the site: no Cloudflare challenge and no age gate, so it
// needs no proxy or cookie handling. Lookup is two requests — search
// (/search?f=all&q=<code>) to find the matching result's detail-page
// link (matched against the <strong> code text in each result card, since
// the search is fuzzy and can return near-miss codes), then a fetch of
// the detail page itself for the metadata panel
// (nav.panel.movie-panel-info > .panel-block, each a "<strong>label:</strong>
// <span class=value>" pair).
package javdb

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/testingbuddies24/HappySorter/internal/scraper"
)

const baseURL = "https://javdb.com"

var (
	runtimeRegex = regexp.MustCompile(`(\d+)\s*分`)
	// ratingRegex pulls the leading score out of JavDB's rating text, e.g.
	// "4.43分, 由2205人評價" -> "4.43". The site's scale tops out at 5, so the
	// value is doubled to match Kodi/Jellyfin's 0-10 rating convention.
	ratingRegex = regexp.MustCompile(`^([\d.]+)`)
)

type Adapter struct {
	client *http.Client
}

func New(client *http.Client) *Adapter {
	return &Adapter{client: client}
}

func (a *Adapter) Name() string { return "javdb" }

func (a *Adapter) Capabilities() scraper.Capabilities {
	return scraper.Capabilities{Kind: scraper.KindAggregator}
}

func (a *Adapter) Lookup(ctx context.Context, code string) (*scraper.Metadata, error) {
	// FC2 detail pages are login-gated (302 to /login even with over18
	// cookies — JavDB made FC2 VIP-only), but the search result card is
	// anonymous and carries the original title and cover, so FC2 lookups
	// stop at the card.
	if strings.HasPrefix(strings.ToUpper(code), "FC2") {
		return a.card(ctx, code)
	}
	detailPath, err := a.search(ctx, code)
	if err != nil {
		return nil, err
	}
	return a.detail(ctx, detailPath)
}

// card answers an FC2 lookup from the search result card alone. JavDB
// indexes FC2 entries without the "PPV" segment ("FC2-1234567"), and the
// fuzzy search only surfaces them when queried in that form — the raw
// "FC2-PPV-1234567" returns nothing but near-miss neighbours.
// fc2Query normalises an FC2 code to JavDB's index form: the site indexes
// "FC2-1234567", never "FC2-PPV-1234567".
func fc2Query(code string) string {
	return strings.Replace(strings.ToUpper(code), "FC2-PPV-", "FC2-", 1)
}

func (a *Adapter) card(ctx context.Context, code string) (*scraper.Metadata, error) {
	query := fc2Query(code)
	searchURL := fmt.Sprintf("%s/search?f=all&q=%s", baseURL, url.QueryEscape(query))

	doc, err := a.get(ctx, searchURL)
	if err != nil {
		return nil, fmt.Errorf("javdb: search %s: %w", searchURL, err)
	}

	return parseCard(doc, query)
}

// parseCard finds the result card whose code exactly matches query and
// reads its title and cover. The match is a string compare because the
// fuzzy search happily returns near-miss codes.
func parseCard(doc *goquery.Document, query string) (*scraper.Metadata, error) {
	meta := &scraper.Metadata{}
	doc.Find(".movie-list .item a.box").EachWithBreak(func(_ int, box *goquery.Selection) bool {
		cardCode := strings.TrimSpace(box.Find(".video-title strong").First().Text())
		if !strings.EqualFold(cardCode, query) {
			return true
		}
		// The anchor's title attribute holds the full original title; the
		// visible .video-title text can be elided with an ellipsis.
		title, ok := box.Attr("title")
		if !ok || strings.TrimSpace(title) == "" {
			videoTitle := strings.TrimSpace(box.Find(".video-title").First().Text())
			title = strings.TrimSpace(strings.TrimPrefix(videoTitle, cardCode))
		}
		meta.Title = title
		if cover, ok := box.Find(".cover img").First().Attr("src"); ok {
			meta.CoverURL = cover
			meta.FanartURL = cover
		}
		return false
	})

	if meta.Title == "" {
		return nil, scraper.ErrNotFound
	}
	return meta, nil
}

// search finds the result card whose code exactly matches (case-
// insensitive) and returns its detail-page path (e.g. "/v/ZY5eq").
func (a *Adapter) search(ctx context.Context, code string) (string, error) {
	searchURL := fmt.Sprintf("%s/search?f=all&q=%s", baseURL, url.QueryEscape(code))

	doc, err := a.get(ctx, searchURL)
	if err != nil {
		return "", fmt.Errorf("javdb: search %s: %w", searchURL, err)
	}

	var detailPath string
	doc.Find(".movie-list .item a.box").EachWithBreak(func(_ int, box *goquery.Selection) bool {
		cardCode := strings.TrimSpace(box.Find(".video-title strong").First().Text())
		if !strings.EqualFold(cardCode, code) {
			return true
		}
		detailPath, _ = box.Attr("href")
		return false
	})
	if detailPath == "" {
		return "", scraper.ErrNotFound
	}
	return detailPath, nil
}

func (a *Adapter) detail(ctx context.Context, path string) (*scraper.Metadata, error) {
	detailURL := baseURL + path

	doc, err := a.get(ctx, detailURL)
	if err != nil {
		return nil, fmt.Errorf("javdb: fetching %s: %w", detailURL, err)
	}

	title := strings.TrimSpace(doc.Find(".origin-title").First().Text())
	if title == "" {
		title = strings.TrimSpace(doc.Find("h2.title strong.current-title").First().Text())
	}
	if title == "" {
		return nil, scraper.ErrNotFound
	}

	meta := &scraper.Metadata{Title: title}

	doc.Find(".movie-panel-info .panel-block").Each(func(_ int, row *goquery.Selection) {
		label := strings.TrimSpace(strings.TrimSuffix(row.Find("strong").First().Text(), ":"))
		value := row.Find("span.value").First()
		switch label {
		case "日期":
			text := strings.TrimSpace(value.Text())
			meta.ReleaseDate = text
			if len(text) >= 4 {
				if year, err := strconv.Atoi(text[:4]); err == nil {
					meta.Year = year
				}
			}
		case "時長":
			if m := runtimeRegex.FindStringSubmatch(value.Text()); m != nil {
				meta.Runtime, _ = strconv.Atoi(m[1])
			}
		case "導演":
			meta.Director = strings.TrimSpace(value.Text())
		case "片商":
			meta.Studio = strings.TrimSpace(value.Text())
		case "系列":
			meta.Series = strings.TrimSpace(value.Text())
		case "評分":
			if m := ratingRegex.FindStringSubmatch(strings.TrimSpace(value.Text())); m != nil {
				if score, err := strconv.ParseFloat(m[1], 64); err == nil {
					meta.Rating = score * 2
				}
			}
		case "類別":
			value.Find("a").Each(func(_ int, link *goquery.Selection) {
				meta.Genres = append(meta.Genres, strings.TrimSpace(link.Text()))
			})
		case "演員":
			value.Find("a").Each(func(_ int, link *goquery.Selection) {
				if link.Next().HasClass("female") {
					meta.Actresses = append(meta.Actresses, strings.TrimSpace(link.Text()))
				}
			})
		}
	})

	if cover, ok := doc.Find("img.video-cover").First().Attr("src"); ok {
		meta.CoverURL = cover
		meta.FanartURL = cover
	}

	return meta, nil
}

func (a *Adapter) get(ctx context.Context, target string) (*goquery.Document, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", target, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, target)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", target, err)
	}
	return doc, nil
}
