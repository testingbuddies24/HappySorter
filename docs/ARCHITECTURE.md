# HappySorter — Architecture

> Status: **Draft v1** (2026-07-13)
> See also: [`SPEC.md`](SPEC.md), [`DEPLOYMENT.md`](DEPLOYMENT.md), [`research/stack-recommendations.md`](research/stack-recommendations.md)

## 1. Bird's-eye view

```
┌──────────────────────────────────────────────────────────────────┐
│                       Docker container                            │
│                                                                   │
│  ┌────────────────┐    ┌──────────────────┐    ┌──────────────┐  │
│  │  FS Watcher    │───▶│  Pipeline Worker │───▶│  Organiser   │  │
│  │  (fsnotify)    │    │  (filter→extract │    │  (Jellyfin   │  │
│  └────────────────┘    │   →scrape→place) │    │   layout)    │  │
│                        └──────────────────┘    └──────────────┘  │
│                                  │                                │
│                                  ▼                                │
│                        ┌──────────────────┐                       │
│                        │  SQLite          │                       │
│                        │  (config+state+  │                       │
│                        │   log+cache)     │                       │
│                        └──────────────────┘                       │
│                                                                   │
│  ┌────────────────┐    ┌──────────────────┐                       │
│  │  HTTP server   │◀───│  GUI (HTMX)      │                       │
│  │  (Fiber/Echo)  │    │  /setup /logs    │                       │
│  └────────────────┘    └──────────────────┘                       │
│                                                                   │
└──────────────────────────────────────────────────────────────────┘
        ▲                       ▲                       ▲
        │                       │                       │
   /config mount           /library mount          /watch mount
   (config.yaml,           (Jellyfin library       (drop folder
    happy-sorter.db)        target)                 for new files)
```

Three volume mounts:
- `/config` — config + DB + logs
- `/library` — the output Jellyfin library (cover.jpg, fanart.jpg, nfo, mp4)
- `/watch` — the input folder the user drops files into

## 2. Component responsibilities

### 2.1 Watcher

- File: `internal/watcher/watcher.go`
- Uses `fsnotify` to watch `/watch` recursively.
- Polling fallback (60 s interval) for filesystems without inotify.
- Emits `NewFile(path string)` events to a channel consumed by the pipeline.

### 2.2 Pipeline worker

- File: `internal/pipeline/pipeline.go`
- One worker goroutine; serial processing keeps things simple.
- State machine per file: `detected → filtering → extracting → scraping → organising → done` (or any → `review`).
- Writes intermediate state to SQLite so a restart can resume.

### 2.3 Rubbish filter

- File: `internal/pipeline/filter.go`
- Pure-function checks against an allow-list (extensions) and deny-list patterns.
- Outputs either `accept` or `route_to_review(reason)`.

### 2.4 Code extractor

- File: `internal/pipeline/code.go`
- Regex match against normalised filename.
- Outputs either `code` or `route_to_review(_unmatched)`.

### 2.5 Scrape manager

- File: `internal/scraper/manager.go`
- Maintains the user-configured ordered list of source adapters.
- Calls `Lookup(code)` on each in turn until one returns a complete metadata.
- Caches positive results in `metadata_cache` table keyed by code.
- Per-source rate limiter (`golang.org/x/time/rate`).

### 2.6 Source adapters

- File: `internal/scraper/<source>/<source>.go`
- Each implements:
  ```go
  type Adapter interface {
      Name() string
      // Capabilities declares what the adapter needs to work, so the
      // manager can skip it early (e.g. no proxy configured but the
      // adapter requires one) and surface a clear reason in the logs.
      Capabilities() Capabilities
      Lookup(ctx context.Context, code string) (*Metadata, error)
  }

  type Capabilities struct {
      NeedsProxy     bool // site is Cloudflare-gated (aggregators)
      NeedsAgeCookie bool // site shows an age gate before content
      Kind           SourceKind // studio | distributor | aggregator
  }
  ```
- One adapter per scrape target site. The HTTP client each adapter uses is
  injected by the manager, already wired with the configured `proxy_url`
  and a per-source cookie jar (see § 4.1).
- Shipped adapters, in default priority order (studio-direct first because
  they are not Cloudflare-gated — see `research/source-test-results.md`):
  `s1`, `ideapocket`, then aggregators `javbus`, `javdb`, `javlibrary`.
  Others pluggable. (`sodprime` and `mgstage` were probed during Milestone
  4a and dropped — both are Japan-only geo-blocked, not Cloudflare-gated,
  so the proxy infra below doesn't help; see `docs/ROADMAP.md` M4a.)

### 2.7 Organiser

- File: `internal/organiser/organiser.go`
- Receives a successful `(code, metadata, source_path)` triple.
- Creates `<CODE> (<YEAR>)/` under `/library`.
- Downloads cover + fanart → `poster.jpg` + `fanart.jpg`.
- Downloads per-actress photos → `actors/`.
- Renames + moves video file.
- Writes `movie.nfo` (Kodi schema).

### 2.8 NFO writer

- File: `internal/nfo/writer.go`
- Pure function: `Write(path string, m *Metadata) error`.
- Emits Kodi movie NFO XML. Jellyfin reads it natively.

### 2.9 Database

- File: `internal/database/database.go`
- SQLite via `modernc.org/sqlite` (no CGO).
- Migrations applied on startup.
- Tables: `config`, `files`, `metadata_cache`, `logs`, `scrape_sources`.

### 2.10 HTTP server

- File: `internal/http/server.go`
- Fiber (or Echo) on port 8080.
- Serves the HTMX-driven setup GUI.
- Endpoints (HTMX fragments and full pages):
  - `GET  /` — dashboard
  - `GET  /setup/folders` `POST /setup/folders`
  - `GET  /setup/sources` `POST /setup/sources` (toggle + reorder)
  - `GET  /setup/rename`  `POST /setup/rename`
  - `GET  /logs` (returns latest N lines, supports `?follow=1`)
  - `GET  /review` `POST /review/retry` `POST /review/delete`
  - `POST /rescan` `POST /pause` `POST /resume`

### 2.11 GUI frontend

- File: `web/templates/*.html`, `web/static/*`
- HTMX + Tailwind (or plain CSS).
- No SPA, no build pipeline beyond Tailwind's standalone CLI if used.
- ~5 templates total — keep it boring.

## 3. Data model

```sql
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
  state        TEXT NOT NULL,           -- detected|filtering|extracting|scrape|organise|done|review_filter|review_unmatched|review_duplicate|failed
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
```

## 4. Scraping flow (multi-source with fallback)

```
code "SSIS-001" arrives
        │
        ▼
manager.Lookup(code)
        │
        ▼
┌───────────────────────────────┐
│ for source in priority order  │  ◀── user-configured, hot-reloadable
└───────────────────────────────┘
        │
        ▼
source.Lookup(code)  (rate-limited per source)
        │
   ┌────┴─────────────┐
   ▼                  ▼
err != nil         success
   │                  │
   │           ┌──────┴────────┐
   │           ▼               ▼
   │      metadata         partial
   │      complete         (missing
   │           │           cover/title)
   │           │              │
   │           ▼              ▼
   │       return         treat as
   │       success        failure → next source
   │
   ▼
all sources failed → mark file as `failed`,
                    leave in place or move to `review/_unmatched/`,
                    surface error in logs GUI
```

**Caching:** once any source returns a complete metadata, it's cached in
`metadata_cache` keyed by code. Subsequent files with the same code (multi-disc
releases) skip scraping entirely.

**Duplicate destination files:** the organiser computes the final video path
before touching anything. If a file already sits there (e.g. the same code was
already organised, or the incoming file is a genuine re-download), it does
**not** silently suffix a new name or overwrite the existing file — the
already-organised release is left completely untouched, and the incoming file
is instead routed to `review/_duplicate/` with `state=review_duplicate`, for
the user to compare and resolve by hand.

### 4.1 HTTP client wiring (proxy + cookies)

Live probing (see `research/source-test-results.md`) showed the aggregators
(JavLibrary, JavDB, JavBus) sit behind Cloudflare and/or an age gate, while
studio-direct sites do not. The manager owns a single HTTP client factory so
each adapter gets a client pre-wired for its needs:

```
manager.clientFor(adapter)
   │
   ├─ if adapter.Capabilities().NeedsProxy && cfg.proxy_url != ""
   │      → route through cfg.proxy_url (HTTP/SOCKS5/CF-Worker)
   │  else if NeedsProxy && proxy_url == ""
   │      → skip adapter, log "needs proxy, none configured"
   │
   ├─ if adapter.Capabilities().NeedsAgeCookie
   │      → attach persistent cookie jar from cookies_dir/<name>.txt
   │      → on first 200-with-age-gate, POST the consent form, save cookie
   │
   └─ always: set browser User-Agent, per-source rate limiter, timeout
```

This keeps Cloudflare/age-gate handling out of the individual adapters —
they only implement `Lookup`, parse HTML, and return `Metadata`. A source
going dark (site adds Cloudflare, changes selectors) is a one-file fix, and
the manager's skip-with-reason logging makes it visible in the GUI.

## 5. Jellyfin folder layout (the contract)

```
<Library Root>/
└── <CODE> (<YEAR>)/
    ├── <CODE> (<YEAR>).<ext>
    ├── poster.jpg                  (cover, portrait)
    ├── fanart.jpg                  (backdrop, landscape)
    ├── backdrop.jpg                (alias of fanart.jpg)
    ├── thumb.jpg                   (small thumb)
    ├── movie.nfo                   (Kodi XML, Jellyfin reads it)
    └── actors/
        └── <actress-slug>.jpg      (per-actress photo)
```

Why this layout works:
- Jellyfin scans any folder; it doesn't *require* `<Title> (<Year>)`, but
  it does use it as a hint when no NFO is present.
- `movie.nfo` is the unambiguous source of truth — we control its contents.
- Per-actress photos in `actors/` show up in Jellyfin's People UI.
- Filename of the video doesn't matter to Jellyfin — it reads from NFO.

## 6. Concurrency model

- **Watcher goroutine:** emits events.
- **Pipeline worker (single):** serial processing — keeps SQLite writes
  trivial, prevents I/O contention on the NAS.
- **HTTP server:** separate goroutines per request (Fiber handles it).
- **Log writer:** separate goroutine drains a channel into SQLite.

If throughput ever becomes a problem, the pipeline worker can be fanned out
to N workers with a keyed mutex on `code` to avoid duplicate scrapes.

## 7. Configuration schema

```yaml
# /config/config.yaml

server:
  port: 8080
  log_level: info        # debug|info|warn|error

paths:
  watch: /watch
  library: /library
  review_filter: /library/review/_filter
  review_unmatched: /library/review/_unmatched
  review_duplicate: /library/review/_duplicate

scraping:
  default_qps: 1.0
  timeout_seconds: 30
  # Optional egress proxy for Cloudflare-gated aggregators. Leave empty to
  # go direct (works from most residential IPs). Accepts an HTTP/SOCKS5 URL
  # or a Cloudflare Worker forwarder base URL. See DEPLOYMENT.md § "Optional:
  # Cloudflare Worker proxy". Adapters with NeedsProxy=true are skipped when
  # this is empty.
  proxy_url: ""
  # Where per-source age-verification cookies are persisted between restarts.
  cookies_dir: /config/cookies

# Default source list, studio-direct first (no Cloudflare), aggregators as
# fallback. Studios reliably resolve their own codes; aggregators cover the
# long tail. Everything ships disabled — the user opts in via the GUI.
sources:
  - name: s1                 # studio: S1 / SSIS — no Cloudflare
    enabled: false
    priority: 1
    qps: 1.0
  - name: ideapocket         # studio: Idea Pocket — no Cloudflare
    enabled: false
    priority: 2
    qps: 1.0
  - name: javbus             # aggregator — age gate, may need proxy
    enabled: false
    priority: 3
    qps: 1.0
  - name: javdb              # aggregator — Cloudflare, needs proxy
    enabled: false
    priority: 4
    qps: 1.0
  - name: javlibrary         # aggregator — most aggressive Cloudflare
    enabled: false
    priority: 5
    qps: 0.5

rename:
  folder_template: "{code} ({year})"
  file_template:   "{code} ({year})"
  unknown_placeholder: "Unknown"
```

All paths are within mounted volumes; if `/watch` doesn't exist on startup,
the watcher logs and waits.

## 8. Failure modes & recovery

| Failure                         | Behavior                                            |
|---------------------------------|-----------------------------------------------------|
| Source site 5xx                 | Retry that source up to 3× with backoff; then next |
| Source site 404 (code not found)| Skip to next source immediately                     |
| Cloudflare challenge (403 "Just a moment")| Adapter needs proxy: if `proxy_url` set, retry via proxy; else skip source, log "Cloudflare-gated, configure proxy_url" |
| Age gate (200 but consent wall) | POST consent form once, persist cookie to `cookies_dir`, retry |
| All sources fail                | Move file to `review/_unmatched/`, log, surface    |
| Destination file already exists | Leave the existing organised release untouched; move the incoming file to `review/_duplicate/`, `state=review_duplicate`, log, surface |
| Cover image download fails      | Generate placeholder poster.jpg; continue           |
| NFO write fails                 | Log error; file marked `failed`; surface in logs   |
| SQLite write fails              | Log error; retry once; if still failing, panic to surface |
| Container restart               | On boot, query DB for `state != 'done'` files, re-enqueue |
| NAS filesystem hiccup           | Watcher logs and retries; no data loss              |

## 9. Security posture

- Single-user; no auth in v1 (assume LAN-only).
- Default bind: `0.0.0.0:8080`. Document how to bind to LAN-only.
- No external network egress except configured scrape sources.
- Magnet URIs are not logged; never appear in API responses.
- Container runs as non-root (UID 1000).
- `read_only` root filesystem where possible; only `/config`, `/library`, `/watch` are writable.
- No new outbound network capabilities added at runtime.

## 10. Update / migration strategy

- `docker pull` new image, restart container.
- DB migrations applied automatically on startup.
- Old `metadata_cache` rows survive image upgrades; old library folders
  survive untouched.
- Backward-compatible within `1.x` releases; `2.x` may require manual
  config migration.

## 11. Observability

- Stdout: structured JSON logs (`slog` with JSON handler).
- `/healthz` endpoint: returns 200 OK + JSON `{version, uptime, queue_size}`.
- `/metrics` (optional, future): Prometheus-format counters
  (files_processed_total, scrape_failures_total{source=...}).
- GUI logs page: tail-style viewer backed by `logs` table.