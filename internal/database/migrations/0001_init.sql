-- config: key/value runtime config (folder paths, port, etc.)
CREATE TABLE config (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

-- files: every file we've ever seen + its current pipeline state
CREATE TABLE files (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  source_path  TEXT NOT NULL,           -- where we found it
  current_path TEXT,                    -- where it is now (may differ)
  state        TEXT NOT NULL,           -- detected|filtering|extracting|scrape|organise|done|review_filter|review_unmatched|failed
  code         TEXT,                    -- extracted JAV code, NULL until extracted
  reason       TEXT,                    -- reason if routed to review
  source       TEXT,                    -- which scrape adapter provided metadata
  metadata_id  INTEGER REFERENCES metadata_cache(id),
  created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(source_path)
);

-- metadata_cache: successful lookups, keyed by code
CREATE TABLE metadata_cache (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  code         TEXT NOT NULL UNIQUE,
  title        TEXT,
  year         INTEGER,
  release_date TEXT,
  studio       TEXT,
  director     TEXT,
  runtime      INTEGER,
  plot         TEXT,
  actresses    TEXT,                    -- JSON array
  genres       TEXT,                    -- JSON array
  cover_path   TEXT,                    -- local path under /library
  fanart_path  TEXT,
  source       TEXT NOT NULL,           -- which adapter populated this
  fetched_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- scrape_sources: ordered list of enabled adapters + their QPS
CREATE TABLE scrape_sources (
  name      TEXT PRIMARY KEY,
  enabled   INTEGER NOT NULL DEFAULT 0,
  priority  INTEGER NOT NULL,           -- lower = tried first
  qps       REAL    NOT NULL DEFAULT 1.0
);

-- logs: ring-buffered to last N entries for the GUI viewer
CREATE TABLE logs (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  level     TEXT NOT NULL,             -- DEBUG|INFO|WARN|ERROR
  message   TEXT NOT NULL,
  fields    TEXT,                      -- JSON
  ts        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
