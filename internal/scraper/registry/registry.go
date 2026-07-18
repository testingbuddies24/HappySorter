// Package registry maps enabled source names in config to concrete
// scraper.Adapter implementations. It exists as a separate package (rather
// than living in internal/scraper itself) because it must import each
// adapter subpackage (e.g. internal/scraper/s1), and those subpackages
// import internal/scraper — putting this here avoids an import cycle.
package registry

import (
	"log/slog"
	"net/http"
	"sort"

	"github.com/testingbuddies24/HappySorter/internal/config"
	"github.com/testingbuddies24/HappySorter/internal/scraper"
	"github.com/testingbuddies24/HappySorter/internal/scraper/ideapocket"
	"github.com/testingbuddies24/HappySorter/internal/scraper/javbus"
	"github.com/testingbuddies24/HappySorter/internal/scraper/javdb"
	"github.com/testingbuddies24/HappySorter/internal/scraper/s1"
)

// BuildAdapters constructs one scraper.Adapter per enabled source in
// sources, in priority order (lowest first). Sources with no adapter
// implemented yet are logged and skipped rather than failing.
//
// javlibrary has no adapter yet: live probing during Milestone 4b found it
// behind a genuine active Cloudflare challenge (unlike javbus and javdb,
// which resolve directly), and there was no proxy available in that
// session to verify real selectors against — see docs/ROADMAP.md M4b. The
// proxy_url config field and its GUI/worker plumbing ship in this
// milestone regardless, ready for whenever the adapter is added.
func BuildAdapters(sources []config.SourceConfig, client *http.Client, logger *slog.Logger) []scraper.Adapter {
	sorted := append([]config.SourceConfig(nil), sources...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Priority < sorted[j].Priority })

	var adapters []scraper.Adapter
	for _, sc := range sorted {
		if !sc.Enabled {
			continue
		}
		switch sc.Name {
		case "s1":
			adapters = append(adapters, s1.New(client))
		case "ideapocket":
			adapters = append(adapters, ideapocket.New(client))
		case "javbus":
			adapters = append(adapters, javbus.New(client))
		case "javdb":
			adapters = append(adapters, javdb.New(client))
		default:
			logger.Warn("source enabled in config but no adapter implemented yet", "source", sc.Name)
		}
	}
	return adapters
}
