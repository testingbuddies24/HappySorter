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
   /config mount           /sorted mount          /download mount
   (config.yaml,           (Jellyfin library       (drop folder
    happy-sorter.db)        target)                 for new files)
```

Three volume mounts:
- `/config` — config + DB + logs
- `/sorted` — the output Jellyfin library (cover.jpg, fanart.jpg, nfo, mp4)
- `/download` — the input folder the user drops files into

## 2. Component responsibilities

### 2.1 Watcher

- File: `internal/downloader/downloader.go`
- Uses `fsnotify` to watch `/download` recursively.
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
- One adapter per scrape target site. The `proxy_url` config field and
  `deploy/cf-worker/worker.js` exist for whichever adapters turn out to
  need them (see § 4.1) — no per-adapter cookie jar was built, since only
  one shipped source ended up needing anything beyond a plain client (see
  below).
- Shipped adapters, in default priority order (studio-direct first because
  they are not Cloudflare-gated — see `research/source-test-results.md`):
  `s1`, `ideapocket`, then aggregators `javbus`, `javdb`. Others pluggable.
  (`sodprime` and `mgstage` were probed during Milestone 4a and dropped —
  both are Japan-only geo-blocked, not Cloudflare-gated, so the proxy
  infra below doesn't help; see `docs/ROADMAP.md` M4a.) Live probing during
  Milestone 4b found `javbus` and `javdb` resolve directly with no proxy
  or age-cookie handling needed at all, contradicting this doc's original
  assumption — only `javlibrary` is genuinely Cloudflare-challenged, and
  its adapter is deferred until a working proxy is available to verify
  selectors against (`docs/ROADMAP.md` M4b).

### 2.7 Organiser

- File: `internal/organiser/organiser.go`
- Receives a successful `(code, metadata, source_path)` triple.
- Creates `<CODE> (<YEAR>)/` under `/sorted`.
- Downloads cover + fanart → `<CODE> (<YEAR>)-poster.jpg` + `<CODE> (<YEAR>)-fanart.jpg`.
- Downloads per-actress photos → `actors/`.
- Renames + moves video file.
- Writes `<CODE> (<YEAR>).nfo` (Kodi schema), sidecars named after the video.

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
  cover_path   TEXT,                    -- local path under /sorted
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

### 4.1 HTTP client wiring (proxy)

Live probing during Milestone 4b (see `docs/ROADMAP.md` M4b) found this
section's original assumption wrong: only JavLibrary is genuinely
Cloudflare-gated at the protocol level. JavBus's age-gate redirect is
cosmetic (the 302 body already has the real page) and JavDB resolves
directly with no challenge — so the originally-planned **per-adapter**,
capability-based `clientFor` routing (only send `NeedsProxy` adapters
through the proxy) was not built.

In practice, though, an individual NAS's IP can still get rate-limited or
flagged by a specific site over time even without a Cloudflare challenge
(observed against `javdb` post-launch — other sources kept working fine).
So instead of gating on adapter capability, `internal/scraper/proxy.go`
wraps the **one shared HTTP client** every adapter already uses in a
`proxyTransport`: when `cfg.scraping.proxy_url` is set, every outgoing
request (any adapter, any site) is rewritten to
`<proxy_url>/?url=<encoded target>` — the pass-through scheme
`deploy/cf-worker/worker.js` implements — and read fresh from the config
store on every request, so a Proxy URL saved via the GUI applies
immediately with no restart. Empty `proxy_url` means every request goes
direct, unchanged.

```
httpClient.Transport = scraper.NewProxyTransport(cfgStore)
   │
   ├─ cfgStore.Get().Scraping.ProxyURL == ""
   │      → pass the request through unmodified
   └─ else
          → rewrite to <proxy_url>/?url=<encoded original request URL>
```

This keeps proxy handling out of the individual adapters entirely — they
only implement `Lookup`, parse HTML, and return `Metadata`, unaware
whether their requests are direct or proxied. Note the Proxy URL field only
speaks this Worker's query-param scheme, not standard HTTP/SOCKS5
forward-proxy protocol.

## 5. Jellyfin folder layout (the contract)

```
<Library Root>/
└── <CODE> (<YEAR>)/
    ├── <CODE> (<YEAR>).<ext>
    ├── <CODE> (<YEAR>)-poster.jpg  (cover, portrait)
    ├── <CODE> (<YEAR>)-fanart.jpg  (backdrop, landscape)
    ├── <CODE> (<YEAR>).nfo         (Kodi XML, Jellyfin reads it)
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
  watch: /download
  library: /sorted
  review_filter: /TBC/_filter
  review_unmatched: /TBC/_unmatched
  review_duplicate: /TBC/_duplicate

scraping:
  default_qps: 1.0
  timeout_seconds: 30
  # Optional egress proxy for when a source starts 403ing your NAS's IP
  # (Cloudflare rate-limit/flag), not only for genuinely Cloudflare-gated
  # sources. Leave empty to go direct. Only accepts a Cloudflare Worker
  # forwarder base URL (deploy/cf-worker/worker.js) — internal/scraper/
  # proxy.go rewrites requests to its <url>/?url=<target> pass-through
  # scheme; a plain HTTP/SOCKS5 proxy URL does not work here. See
  # DEPLOYMENT.md § 4a. Applied uniformly to every adapter's shared HTTP
  # client, re-read live on every request (§ 4.1) — no restart needed.
  proxy_url: ""
  # Reserved for a future per-source age-verification cookie jar; unused —
  # no shipped adapter needs one (see § 2.6).
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
  - name: javbus             # aggregator — resolves directly, no proxy needed
    enabled: false
    priority: 3
    qps: 1.0
  - name: javdb              # aggregator — resolves directly, no proxy needed
    enabled: false
    priority: 4
    qps: 1.0
  - name: javlibrary         # aggregator — Cloudflare-gated; adapter not yet implemented
    enabled: false
    priority: 5
    qps: 0.5

rename:
  folder_template: "{code}"
  file_template:   "{code}"
  unknown_placeholder: "Unknown"
```

All paths are within mounted volumes; if `/download` doesn't exist on startup,
the watcher logs and waits.

## 8. Failure modes & recovery

| Failure                         | Behavior                                            |
|---------------------------------|-----------------------------------------------------|
| Source site 5xx                 | Retry that source up to 3× with backoff; then next |
| Source site 404 (code not found)| Skip to next source immediately                     |
| Cloudflare challenge (403 "Just a moment")| Not yet implemented — no shipped adapter hits this (see § 4.1); reserved for whenever the JavLibrary adapter is added |
| Age gate (200 but consent wall) | Not applicable to any shipped adapter — JavBus's redirect-to-age-gate is cosmetic and doesn't need a consent POST (see § 2.6) |
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
- `read_only` root filesystem where possible; only `/config`, `/sorted`, `/download` are writable.
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