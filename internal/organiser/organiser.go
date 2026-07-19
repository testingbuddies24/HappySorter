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
	"context"
	"fmt"
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
	if m.CoverURL == "" || o.download(ctx, m.CoverURL, filepath.Join(dir, posterName)) != nil {
		if err := writePlaceholderPoster(filepath.Join(dir, posterName), m.Code); err != nil {
			return "", fmt.Errorf("writing placeholder poster: %w", err)
		}
	}
	art := nfo.Artwork{Poster: posterName}
	if m.FanartURL != "" {
		fanartName := base + "-fanart.jpg"
		if err := o.download(ctx, m.FanartURL, filepath.Join(dir, fanartName)); err == nil {
			art.Fanart = fanartName
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

func (o *Organiser) download(ctx context.Context, imgURL, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imgURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")
	// Some image CDNs (e.g. JavBus) hotlink-protect on Referer; a same-origin
	// referer satisfies that check without needing the exact source page.
	if parsed, err := url.Parse(imgURL); err == nil {
		req.Header.Set("Referer", parsed.Scheme+"://"+parsed.Host+"/")
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d fetching %s", resp.StatusCode, imgURL)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}
