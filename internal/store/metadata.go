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
		       actresses, genres, cover_path, fanart_path, source
		FROM metadata_cache WHERE code = ?`, code)

	var m scraper.Metadata
	var actressesJSON, genresJSON sql.NullString
	m.Code = code
	err := row.Scan(&m.Title, &m.Year, &m.ReleaseDate, &m.Studio, &m.Director,
		&m.Runtime, &m.Plot, &actressesJSON, &genresJSON, &m.CoverURL, &m.FanartURL, &m.Source)
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

// Put caches m, keyed by its code. cover_path/fanart_path store the
// source URL for now (not yet a local /library path as the schema comment
// envisions) — the organiser re-downloads on every Organise call, so a
// cache hit only saves the HTML scrape, not the image fetch.
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
			 actresses, genres, cover_path, fanart_path, source, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(code) DO UPDATE SET
			title=excluded.title, year=excluded.year, release_date=excluded.release_date,
			studio=excluded.studio, director=excluded.director, runtime=excluded.runtime,
			plot=excluded.plot, actresses=excluded.actresses, genres=excluded.genres,
			cover_path=excluded.cover_path, fanart_path=excluded.fanart_path,
			source=excluded.source, fetched_at=excluded.fetched_at`,
		m.Code, m.Title, m.Year, m.ReleaseDate, m.Studio, m.Director, m.Runtime, m.Plot,
		string(actressesJSON), string(genresJSON), m.CoverURL, m.FanartURL, m.Source, time.Now())
	return err
}
