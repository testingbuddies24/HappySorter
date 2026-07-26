// Package organiser lays scraped files out into the Jellyfin-recognised
// folder structure (docs/ARCHITECTURE.md § 2.7, § 5). Sidecars are named
// after the video file so Jellyfin pairs them by basename:
//
//	<CODE> (<YEAR>)/<CODE> (<YEAR>).<ext>
//	<CODE> (<YEAR>)/<CODE> (<YEAR>)-poster.jpg
//	<CODE> (<YEAR>)/<CODE> (<YEAR>)-fanart.jpg
//	<CODE> (<YEAR>)/<CODE> (<YEAR>).nfo
package organiser

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/testingbuddies24/HappySorter/internal/config"
	"github.com/testingbuddies24/HappySorter/internal/fsutil"
	"github.com/testingbuddies24/HappySorter/internal/nfo"
	"github.com/testingbuddies24/HappySorter/internal/scraper"
)

// frontCoverRatio is cropWidth/height for isolating the front-cover panel out
// of the wraparound package scan every current source serves as CoverURL
// (back-cover blurb + spine + front cover, all in one image). Derived from a
// known-good reference: an older tool's output had a 378x538 front-only
// poster next to an 800x538 full package image for the same release
// (378/538 = 0.7026), and the same ratio applied to an 800x438 copy of that
// same title from a different source lands cleanly on the front art too —
// the crop is relative to height, not a fixed pixel offset, so it survives
// sources serving the package scan at different resolutions.
const frontCoverRatio = 378.0 / 538.0

// cropFrontCover isolates the front-cover panel from a wraparound package
// scan: the front art sits in the rightmost frontCoverRatio*height pixels.
// Returns re-encoded JPEG bytes, or an error if raw isn't a decodable image
// or is already narrower than the computed crop (caller should fall back to
// writing raw unmodified rather than lose the cover entirely).
func cropFrontCover(raw []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decoding cover image: %w", err)
	}

	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	cropW := int(float64(h) * frontCoverRatio)
	if cropW <= 0 || cropW >= w {
		return nil, fmt.Errorf("cover image %dx%d too narrow to crop", w, h)
	}

	srcRect := image.Rect(b.Max.X-cropW, b.Min.Y, b.Max.X, b.Max.Y)
	dst := image.NewRGBA(image.Rect(0, 0, cropW, h))
	draw.Draw(dst, dst.Bounds(), img, srcRect.Min, draw.Src)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 90}); err != nil {
		return nil, fmt.Errorf("encoding cropped cover: %w", err)
	}
	return buf.Bytes(), nil
}

type Organiser struct {
	cfgStore *config.Store
	client   *http.Client
}

// New builds an Organiser that reads the library path and rename templates
// fresh from cfgStore on every call, so GUI edits to either take effect
// without a restart.
func New(cfgStore *config.Store, client *http.Client) *Organiser {
	return &Organiser{cfgStore: cfgStore, client: client}
}

// DuplicateError is returned by Organise when a file already sits at the
// computed video destination. The caller should route the incoming file
// somewhere for the user to handle manually, rather than silently
// suffixing a new name or overwriting the existing one.
type DuplicateError struct {
	ExistingPath string
}

func (e *DuplicateError) Error() string {
	return fmt.Sprintf("a file already exists at %s", e.ExistingPath)
}

// Organise moves videoPath into the release folder for m and writes its
// poster/fanart/NFO alongside it. Returns the video's final path.
func (o *Organiser) Organise(ctx context.Context, m *scraper.Metadata, videoPath string) (string, error) {
	cfg := o.cfgStore.Get()
	base := o.renderName(cfg.Rename, cfg.Rename.FileTemplate, m)
	dir := filepath.Join(cfg.Paths.Library, o.renderName(cfg.Rename, cfg.Rename.FolderTemplate, m))
	dest := filepath.Join(dir, base+strings.ToLower(filepath.Ext(videoPath)))

	// Treat an existing release folder as a duplicate: the code is already in
	// the library, so route the newcomer aside rather than merging a second
	// video (and overwriting sidecars) into the same folder.
	if _, err := os.Stat(dir); err == nil {
		return "", &DuplicateError{ExistingPath: dir}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating library folder: %w", err)
	}

	// Sidecars share the video's basename so Jellyfin pairs them (e.g.
	// "MIDA-678 (2026)-poster.jpg" next to "MIDA-678 (2026).mp4").
	posterName := base + "-poster.jpg"
	fanartName := base + "-fanart.jpg"
	art := nfo.Artwork{Poster: posterName}

	raw, err := o.downloadCover(ctx, m.CoverURL)
	if err != nil {
		if err := writePlaceholderPoster(filepath.Join(dir, posterName), m.Code); err != nil {
			return "", fmt.Errorf("writing placeholder poster: %w", err)
		}
	} else {
		// Every current source's CoverURL is a wraparound package scan (back
		// blurb + spine + front cover in one image) — write it whole as
		// fanart, and isolate the front panel as the poster so Jellyfin
		// doesn't show the combined scan as the item's cover.
		if err := os.WriteFile(filepath.Join(dir, fanartName), raw, 0o644); err != nil {
			return "", fmt.Errorf("writing fanart: %w", err)
		}
		art.Fanart = fanartName

		poster := raw
		if cropped, err := cropFrontCover(raw); err == nil {
			poster = cropped
		}
		if err := os.WriteFile(filepath.Join(dir, posterName), poster, 0o644); err != nil {
			return "", fmt.Errorf("writing poster: %w", err)
		}
	}

	if err := nfo.Write(filepath.Join(dir, base+".nfo"), m, art); err != nil {
		return "", fmt.Errorf("writing nfo: %w", err)
	}

	if err := fsutil.MoveFile(videoPath, dest); err != nil {
		return "", fmt.Errorf("moving video: %w", err)
	}

	return dest, nil
}

func (o *Organiser) renderName(rename config.RenameConfig, template string, m *scraper.Metadata) string {
	year := rename.UnknownPlaceholder
	if m.Year > 0 {
		year = strconv.Itoa(m.Year)
	}
	r := strings.NewReplacer("{code}", m.Code, "{year}", year)
	return r.Replace(template)
}

// downloadCover fetches coverURL, treating an empty URL the same as a
// download failure so callers have a single fallback path (placeholder
// poster, no fanart).
func (o *Organiser) downloadCover(ctx context.Context, coverURL string) ([]byte, error) {
	if coverURL == "" {
		return nil, fmt.Errorf("no cover URL")
	}
	return o.download(ctx, coverURL)
}

// download fetches imgURL and returns its raw bytes (the caller decides
// whether/how to write them out — see Organise, which writes the same
// fetched cover as both the full fanart and a cropped poster).
func (o *Organiser) download(ctx context.Context, imgURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imgURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")
	// Some image CDNs (e.g. JavBus) hotlink-protect on Referer; a same-origin
	// referer satisfies that check without needing the exact source page.
	if parsed, err := url.Parse(imgURL); err == nil {
		req.Header.Set("Referer", parsed.Scheme+"://"+parsed.Host+"/")
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d fetching %s", resp.StatusCode, imgURL)
	}

	return io.ReadAll(resp.Body)
}
