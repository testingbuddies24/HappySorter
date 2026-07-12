# Research: JAV metadata standards

> Source: HappySorter research agent #2 (run 2026-07-13)
> Status: complete

## 1. What is a JAV code?

A JAV (Japanese Adult Video) code is a studio-assigned alphanumeric identifier
printed on every release, format `{PREFIX}-{NUMBER}`:

- **Prefix** — 3-4 letters (sometimes 2) denoting the studio / label / series.
- **Suffix number** — 3-5 digits, assigned sequentially per title.

Examples: `SSIS-001`, `ABW-123`, `MIDE-456`, `HEY-067`, `CJOD-031`.

Variant codes include:
- `5ONE-*` (rental series),
- `ONSD-*` (compilation series),
- special numeric dates for some uncensored re-releases.

### Parsing regex (initial draft)

```regex
^([A-Z0-9]{2,5})-(\d{2,5})$
```

Must be case-insensitive in practice — many files end up lowercase after
filesystem shuffles. Real-world matching needs preprocessing (strip release
group tags like `-CH`, `-UC`, `-JP`, common torrent release suffixes).

## 2. Major studios and prefix table (initial seed)

| Prefix      | Studio / Label                                       |
|-------------|------------------------------------------------------|
| `S1` / `SOFT` | Soft on Demand / S1 NO.1 STYLE                    |
| `SSIS`      | Ssis (S1 subsidiary, uncensored)                     |
| `ONED`      | Oned (Soft on Demand line)                           |
| `PSD`       | PSD (Soft on Demand line)                            |
| `ABW`       | Absolut Mediens (German JAV)                         |
| `MIDE`      | Mood Works                                           |
| `DPMI`      | DPM (Delta / Dream)                                  |
| `IES`       | Idea Pocket                                          |
| `E-BODY`    | E-Body                                               |
| `KMU`       | Kuku (censored)                                      |
| `CJOD`      | Crystal Clear (uncensored)                           |
| `GAO`       | G-Area                                               |
| `NHDTV`     | New Hot Entertainment                                |
| `RBD`       | Ruby Bird                                            |
| `JUFD`      | Judas                                                |
| `MXGS`      | Max Green                                            |
| `HND`       | Hand (French JAV)                                    |
| `KTKL`      | KTK (Kitakaya)                                       |
| `FAA`       | F&A                                                  |
| `TT`        | TT (Tokyo Tel)                                       |
| `MGM`       | Mediamix                                             |
| `FPW`       | First Star                                           |
| `UFO`       | UFO (umbrella brand)                                 |

> There are 100+ active prefixes. This list is a starter — the scraper
> should not hard-code studio→prefix mapping; the metadata source provides it.
> The table exists only for diagnostic logs ("file `XYZ-123` matched against
> studio `???` — prefix unknown").

## 3. Standard metadata fields

The schema HappySorter will normalize metadata into (regardless of source):

| Field         | Type        | Notes                                       |
|---------------|-------------|---------------------------------------------|
| `code`        | string      | Primary key, e.g. `SSIS-001`                |
| `title`       | string      | Full title (often JP + romanised)           |
| `release_date`| ISO date    | YYYY-MM-DD                                  |
| `studio`      | string      | Producing studio/label                      |
| `label`       | string      | Sub-label                                   |
| `director`    | string[]    | 1+ names                                    |
| `runtime`     | int (min)   | Length in minutes                           |
| `actresses`   | string[]    | Stage names                                 |
| `genres`      | string[]    | Categories                                  |
| `cover_url`   | string      | URL to cover image                          |
| `sample_urls` | string[]    | Sample screenshot URLs (for fanart/backdrop)|
| `magnet_uri`  | string?     | Private-use reference only                  |
| `torrent_file`| string?     | Path/name (private-use reference only)      |
| `rating`      | float?      | e.g. JavLibrary community rating            |
| `series`      | string?     | Series name (when applicable)               |
| `notes`       | string?     | Free-text user notes                        |
| `created_at`  | timestamp   | DB-managed                                  |
| `updated_at`  | timestamp   | DB-managed                                  |

## 4. Reference data sources

| Site              | URL                | Strengths                          | Weaknesses                              |
|-------------------|--------------------|------------------------------------|-----------------------------------------|
| **JavLibrary**    | javlibrary.com     | Largest, community-driven, oldest  | Heavy anti-bot, no API                  |
| **JavBus**        | javbus.com         | Generous with scraping             | Chinese-hosted; CDN can rot             |
| **JavDB**         | javdb.com          | Fast search, modern UI            | Some rate-limit; hash-based image names |
| **DMM / FANZA**   | dmm.co.jp / fanza.com | Authoritative licensed metadata | CAPTCHA on bulk, no public API          |
| **R18.com**       | r18.com            | Distributor; strong on new titles  | ToS restricts scraping                  |

**Anti-scraping posture:**
- JavLibrary: blocks automated requests; needs UA rotation, rate limiting, session cookies.
- JavBus: more permissive; no visible bot detection on static pages; can block after burst.
- JavDB: relatively permissive; some rate-limiting on search.
- DMM/FANZA: strong anti-bot; CAPTCHA for bulk.

**No public APIs** exist. All sources require HTML scraping. → URLs and
selectors will change; adapters must be isolated and easy to swap.

## 5. Cover / preview image conventions

| Site       | Image host pattern                        | Naming                  |
|------------|-------------------------------------------|-------------------------|
| JavLibrary | `imgcdn.javlibrary.com/cover/...`         | Sequential              |
| JavBus     | `www.javbus.com/pics/cover/<id>.jpg`      | Sequential              |
| JavDB      | `cdn.javdb.com/covers/<hash>.jpg`         | Hash-based (irreversible without DB) |
| DMM        | `pics.dmm.co.jp/mono/movie/<id>/<id>.jpg` | Content-ID-based        |
| R18        | `image.r18.com/...`                       | Various                 |

**Implication for HappySorter:** always **download and store images locally**
rather than referencing CDN URLs. URL rot is the #1 reason image refs die.

## 6. Magnet link context

A magnet URI is a peer-to-peer download reference (`magnet:?xt=urn:btih:<hash>&...`).
It does not contain the file itself — only the content hash used by DHT peers.

Why a personal library tool stores them:
- Personal offline reference (re-acquire lost files).
- Correlation with JAV code for findability.
- Not for redistribution; never exposed via public API.

**Security / privacy posture for HappySorter:**
- Store magnets in a private DB column.
- Never log full URIs (log only the btih hash prefix).
- No public sharing endpoint.
- Document explicitly: for personal backup only; unauthorized download is
  illegal in most jurisdictions.

## 7. Stack recommendations for Docker-on-NAS

### Constraints
- Synology DSM 7.x + QNAP QTS both run Docker on x86_64 and ARM.
- Typical NAS RAM: 512 MB – 2 GB. Anything eating >500 MB at idle is bad.
- Users want **one docker run command** or a docker-compose.yml.

### Recommended stack (Go-based)

| Layer | Choice | Why |
|---|---|---|
| Language | Go 1.21+ | Single binary, low RAM, cross-compile for ARM+x86 |
| DB | SQLite via `modernc.org/sqlite` (pure Go, no CGO) | Zero-config, single file, no daemon |
| Web framework | Fiber or Echo | Lightweight, fast |
| Frontend | Go templates + HTMX | Progressive enhancement, no JS framework |
| Docker | `FROM alpine` or `FROM scratch` | ~30–80 MB final image |
| Multi-arch | `docker buildx` for `linux/amd64,linux/arm64` | Single image covers all NAS |

### Alternative stack (Python)

| Layer | Choice | Why |
|---|---|---|
| Language | Python 3.11+ | Richer scraping ecosystem (BeautifulSoup, httpx) |
| DB | SQLite via `aiosqlite` | Async-friendly |
| Web | FastAPI + Uvicorn + Jinja2 | Easy templating |
| Docker | `FROM python:alpine` | ~80–120 MB |

### Why NOT Postgres
- Requires running server process, more RAM.
- Overkill for personal libraries (<10K items typical).
- SQLite scales to millions of rows; purpose-built for embedded use.

### Why HTMX over React/Vue
- Keeps frontend footprint tiny (one ~14 KB script).
- Server renders HTML; no SPA build pipeline.
- Perfect for a setup-only GUI.

## 8. Key gaps in existing OSS (per research)

1. No lightweight all-in-one Docker image (scrape + DB + web UI <100 MB).
2. No unified multi-site scraper with auto-fallback when one site changes.
3. No CLI-first tool with companion setup GUI targeting NAS use.
4. Most scrapers don't store images locally → image URLs rot.
5. No actress deduplication across studios (stage name variants).

→ HappySorter's value prop is to fill these gaps.

## 9. Suggested project layout (Go-based)

```
happy-sorter/
├── cmd/server/              # Main web server entrypoint
├── internal/
│   ├── scraper/             # Multi-site scraper adapters
│   │   ├── javlibrary/
│   │   ├── javbus/
│   │   ├── javdb/
│   │   └── common/          # Shared adapter interface + Metadata type
│   ├── database/            # SQLite schema + migrations
│   ├── models/              # Domain types
│   ├── pipeline/            # Auto-detect → filter → scrape → organise
│   ├── watcher/             # Filesystem watcher
│   ├── handlers/            # HTTP handlers (setup GUI + log viewer)
│   └── nfo/                 # Kodi/Jellyfin NFO writer
├── web/
│   ├── templates/           # Go HTML templates
│   └── static/              # CSS/JS (HTMX)
├── migrations/              # SQL migrations
├── configs/                 # Sample configs
├── docs/
├── Dockerfile
├── docker-compose.yml
├── go.mod
└── README.md
```

## 10. Direct URLs to verify in future research

- https://www.javlibrary.com/
- https://www.javbus.com/
- https://javdb.com/
- https://www.dmm.co.jp/
- https://www.r18.com/
- Existing OSS to study:
  - `Jellyfin.Plugin.Jav*` (C# plugin; mature)
  - `javlibrary-scraper` (Python; multiple forks)
  - `AV-Organizer` (Go single-binary)