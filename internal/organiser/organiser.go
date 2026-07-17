// Package organiser lays scraped files out into the Jellyfin-recognised
// folder structure (docs/ARCHITECTURE.md § 2.7, § 5):
//
//	<CODE> (<YEAR>)/<CODE> (<YEAR>).<ext>
//	<CODE> (<YEAR>)/poster.jpg
//	<CODE> (<YEAR>)/fanart.jpg
//	<CODE> (<YEAR>)/backdrop.jpg  (alias of fanart.jpg)
//	<CODE> (<YEAR>)/movie.nfo
package organiser

import (
	"context"
	"fmt"
	"io"
	"net/http"
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
	dir := filepath.Join(cfg.Paths.Library, o.renderName(cfg.Rename, cfg.Rename.FolderTemplate, m))
	fileName := o.renderName(cfg.Rename, cfg.Rename.FileTemplate, m) + strings.ToLower(filepath.Ext(videoPath))
	dest := filepath.Join(dir, fileName)

	if _, err := os.Stat(dest); err == nil {
		return "", &DuplicateError{ExistingPath: dest}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating library folder: %w", err)
	}

	if m.CoverURL != "" {
		if err := o.download(ctx, m.CoverURL, filepath.Join(dir, "poster.jpg")); err != nil {
			return "", fmt.Errorf("downloading cover: %w", err)
		}
	}
	if m.FanartURL != "" {
		if err := o.download(ctx, m.FanartURL, filepath.Join(dir, "fanart.jpg")); err != nil {
			return "", fmt.Errorf("downloading fanart: %w", err)
		}
		if err := copyFile(filepath.Join(dir, "fanart.jpg"), filepath.Join(dir, "backdrop.jpg")); err != nil {
			return "", fmt.Errorf("aliasing backdrop: %w", err)
		}
	}

	if err := nfo.Write(filepath.Join(dir, "movie.nfo"), m); err != nil {
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

func (o *Organiser) download(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d fetching %s", resp.StatusCode, url)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

func copyFile(src, dest string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o644)
}
