package store

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/testingbuddies24/HappySorter/internal/scraper"
)

// MetadataStore wraps the `metadata_cache` table so multi-disc releases
// (same code, multiple video files) skip re-scraping on subsequent lookups
// (docs/ARCHITECTURE.md § 3, § 4).
type MetadataStore struct {
	db *sql.DB
}

func NewMetadataStore(db *sql.DB) *MetadataStore {
	return &MetadataStore{db: db}
}

// Get returns cached metadata for code, and whether it was found.
func (s *MetadataStore) Get(code string) (*scraper.Metadata, bool, error) {
	row := s.db.QueryRow(`
		SELECT title, year, release_date, studio, director, runtime, plot,
		       actresses, genres, cover_path, fanart_path, source,
		       COALESCE(series, ''), COALESCE(label, ''), COALESCE(rating, 0)
		FROM metadata_cache WHERE code = ?`, code)

	var m scraper.Metadata
	var actressesJSON, genresJSON sql.NullString
	m.Code = code
	err := row.Scan(&m.Title, &m.Year, &m.ReleaseDate, &m.Studio, &m.Director,
		&m.Runtime, &m.Plot, &actressesJSON, &genresJSON, &m.CoverURL, &m.FanartURL, &m.Source,
		&m.Series, &m.Label, &m.Rating)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if actressesJSON.Valid {
		json.Unmarshal([]byte(actressesJSON.String), &m.Actresses)
	}
	if genresJSON.Valid {
		json.Unmarshal([]byte(genresJSON.String), &m.Genres)
	}
	return &m, true, nil
}

// Put caches m, keyed by its code. cover_path/fanart_path hold the source
// URL, not a local path — that's deliberate: this table is the scraped-
// metadata cache (skips re-running the HTML scrape for multi-disc/retry
// lookups of the same code), not an image cache. The downloaded image
// bytes are cached separately on disk by the organiser (see
// internal/organiser), keyed by code, so a metadata cache hit doesn't imply
// a redundant image download.
func (s *MetadataStore) Put(m *scraper.Metadata) error {
	actressesJSON, err := json.Marshal(m.Actresses)
	if err != nil {
		return err
	}
	genresJSON, err := json.Marshal(m.Genres)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
		INSERT INTO metadata_cache
			(code, title, year, release_date, studio, director, runtime, plot,
			 actresses, genres, cover_path, fanart_path, source, series, label, rating, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(code) DO UPDATE SET
			title=excluded.title, year=excluded.year, release_date=excluded.release_date,
			studio=excluded.studio, director=excluded.director, runtime=excluded.runtime,
			plot=excluded.plot, actresses=excluded.actresses, genres=excluded.genres,
			cover_path=excluded.cover_path, fanart_path=excluded.fanart_path,
			source=excluded.source, series=excluded.series, label=excluded.label,
			rating=excluded.rating, fetched_at=excluded.fetched_at`,
		m.Code, m.Title, m.Year, m.ReleaseDate, m.Studio, m.Director, m.Runtime, m.Plot,
		string(actressesJSON), string(genresJSON), m.CoverURL, m.FanartURL, m.Source,
		nullIfEmpty(m.Series), nullIfEmpty(m.Label), m.Rating, time.Now())
	return err
}
