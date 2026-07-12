# Research: stack recommendations

> Source: HappySorter research agent #2 (run 2026-07-13)
> Status: complete — provisional, may revise after spike

## TL;DR

| Layer         | Recommendation                                  |
|---------------|--------------------------------------------------|
| Language      | **Go 1.21+**                                     |
| Web framework | Fiber or Echo                                    |
| Templates     | Go html/template + HTMX                          |
| DB            | SQLite (via `modernc.org/sqlite`, pure-Go)       |
| Frontend      | HTMX + Tailwind (or vanilla CSS)                 |
| Watcher       | `fsnotify` (cross-platform)                      |
| HTTP client   | `net/http` + `colly` (optional) for scraping     |
| Docker        | `FROM alpine` final image, multi-arch            |
| Logging       | `log/slog` (stdlib) → SQLite + stdout            |

## 1. Why Go over Python / Node

| Criterion           | Go                  | Python             | Node               |
|---------------------|---------------------|--------------------|--------------------|
| Single binary       | ✅                  | ❌ (needs runtime) | ❌                 |
| Cross-compile ARM+x86| ✅ trivial          | ⚠️ musl work       | ⚠️                  |
| Idle RAM            | ~10–30 MB           | ~80–120 MB         | ~80–150 MB         |
| Scraping libs       | `net/http` + colly  | BeautifulSoup etc. | puppeteer (heavy)  |
| Hot-reload UI work  | Compile cycle ~5s   | Reload ~1s         | Reload ~1s         |
| Docker image size   | 30–80 MB            | 80–120 MB          | 150+ MB            |
| Maintenance burden  | Statically typed    | Dynamic            | Dynamic            |

Go wins on the dimensions that matter most for a NAS appliance: low RAM,
single binary, cross-compile, small image. Python wins on ecosystem ergonomics
for HTML scraping — we mitigate with `colly` + plain regex in Go.

## 2. Why SQLite over Postgres

- Personal libraries are <10K items → SQLite scales to millions.
- Single file → trivial backup (`cp happy-sorter.db backup.db`).
- No daemon → one fewer thing to crash in the container.
- `modernc.org/sqlite` is pure-Go → no CGO dependency → tiny alpine image.
- Migration path to Postgres is straightforward via GORM/sqlc if needed.

## 3. Why HTMX over React/Vue/Svelte

- One ~14 KB script tag.
- Server renders HTML → no SPA build pipeline.
- Perfect for a setup GUI: form posts, page updates, list refreshes.
- Easy to maintain; any backend dev can read HTMX templates.

If interactivity requirements grow beyond HTMX's sweet spot (e.g., live log
streaming with virtual scroll), add Alpine.js. Avoid reaching for a SPA.

## 4. Folder watcher

`fsnotify` is the de-facto Go filesystem watcher. Cross-platform (Linux
inotify, macOS FSEvents, Windows ReadDirectoryChangesW). Pure Go, no cgo.

Caveat: some NAS filesystems (NFS, SMB shares) don't generate inotify events
reliably. Fall back to periodic polling if `fsnotify` doesn't deliver events.

## 5. Scraping HTTP client

- `net/http` for simple GETs.
- `github.com/gocolly/colly` if we want a structured scraper framework
  (rate limiting, cookie jars, parallelism).
- Per-adapter: hard-cap QPS, configurable via env.

## 6. NFO writer

Kodi movie NFO is a stable XML schema (since 2009). Jellyfin reads it
unchanged. No need to invent our own format.

Reference fields HappySorter will populate:
```xml
<?xml version="1.0" encoding="UTF-8"?>
<movie>
  <title>SSIS-001</title>
  <originaltitle>...</originaltitle>
  <year>2018</year>
  <plot>...</plot>
  <runtime>120</runtime>
  <mpaa>XXX</mpaa>
  <studio>S1 NO.1 STYLE</studio>
  <director>...</director>
  <actor>
    <name>...</name>
    <thumb>actors/....jpg</thumb>
  </actor>
  ...
  <genre>...</genre>
  <id>SSIS-001</id>
  <uniqueid type="jav" default="true">SSIS-001</uniqueid>
</movie>
```

## 7. Docker

```dockerfile
# Build stage
FROM golang:1.21-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags='-s -w' -o /out/happy-sorter ./cmd/server

# Runtime stage
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/happy-sorter /usr/local/bin/happy-sorter
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/happy-sorter"]
```

Multi-arch build:
```bash
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t ghcr.io/<owner>/happy-sorter:latest \
  --push .
```

## 8. What we are NOT using (and why)

| Tech          | Why not                                |
|---------------|-----------------------------------------|
| Postgres      | Overkill; SQLite is enough             |
| Redis         | No need; in-process state is fine       |
| Kafka / queue | Single-process pipeline; no queue needed|
| React/Vue     | HTMX covers it; SPA is overkill        |
| Kubernetes    | User runs this on one NAS box           |
| OAuth / SSO   | Single-user, single-household           |

## 9. Open questions to resolve before implementation

1. Do we need to support non-`colly` headless-browser scraping for JS-heavy sites? (Adds 100+ MB to image.)
2. Do we want a CLI mode alongside the GUI? (Probably yes; CLI for power users, GUI for setup.)
3. Do we ship a default scrape-source priority list, or empty + user-configured? (Empty, with documentation — see legal/ToS note.)
4. Update strategy: `latest` tag, versioned tags, or watchtower-style auto-update? (User's call.)