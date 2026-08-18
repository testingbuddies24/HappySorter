<div align="center">

<img src="icon.svg" width="104" alt="HappySorter logo: a smirk face on an indigo tile">

# HappySorter

Self-hosted, Docker-deployable organizer for personal JAV media libraries, with first-class [Jellyfin](https://jellyfin.org/) compatibility.

[![CI](https://github.com/testingbuddies24/HappySorter/actions/workflows/ci.yml/badge.svg)](https://github.com/testingbuddies24/HappySorter/actions/workflows/ci.yml)
[![Release](https://github.com/testingbuddies24/HappySorter/actions/workflows/release.yml/badge.svg)](https://github.com/testingbuddies24/HappySorter/actions/workflows/release.yml)
[![Latest release](https://img.shields.io/github/v/release/testingbuddies24/HappySorter)](https://github.com/testingbuddies24/HappySorter/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

</div>

Drop a video file into a watched folder. HappySorter parses the JAV code from the filename, scrapes metadata from multiple sources (with ordered fallback and field-level merging), and lays out the file into a Jellyfin-recognized folder: `<CODE>/<CODE>.mp4` + `<CODE>-poster.jpg` + `<CODE>-fanart.jpg` + `<CODE>.nfo` (folder/file naming is a configurable template, e.g. to include the year).

Runs as a single Docker container on a NAS (Synology, QNAP, anything x86_64 / arm64). The setup GUI is a one-time configuration tool — your actual library is browsed in Jellyfin.

## Why

The legacy tool at  is a 2015 Windows .NET file-renamer whose backend API has since died. HappySorter is the modern, self-hosted, multi-source equivalent — portable, Docker-native, and not at the mercy of any single website.

## Features

- 📁 **Folder watcher** — drop files in `/download`, they appear organised in `/sorted`.
- 🗑️ **Rubbish filter** — junk files (`.url`, `.txt`, samples, trailers) routed to `/TBC/_filter`, one-click bulk delete from the GUI.
- 🔎 **Multi-source scrape with field-level merging** — S1, IdeaPocket, JavBus, JavDB, MissAV, JavMenu tried in priority order; the first complete result is filled in with whatever fields (plot, genres, series, rating, label, ...) it's missing from the next source, instead of a single source winning wholesale. MissAV and JavMenu are FC2 specialists — they picked up FC2 lookups after JavBus dropped its FC2 catalogue.
- 🎬 **Jellyfin-compatible output** — `movie.nfo` (title, plot, genres, rating, series, distributor label, ...) + cropped front-cover poster + full-scan fanart.
- 📊 **Live dashboard** — last 60 minutes of pipeline activity, streamed in real time.
- 🔁 **TBC review queue** — retry, delete, or bulk-delete rejected/unmatched/duplicate files from the GUI, with a "Refresh from disk" action that reconciles the queue after manual file edits.
- 🐳 **One container** — ~30–80 MB final image, multi-arch (`linux/amd64`, `linux/arm64`), idle RAM ≤ 100 MB.
- 🖥️ **Web GUI for setup** — configure folders, sources, rename template, view logs.
- 🔄 **Crash-safe** — pipeline state in SQLite; container restart resumes from where it left off.

## Quickstart

```bash
docker run -d \
  --name happy-sorter \
  --restart unless-stopped \
  -p 8080:8080 \
  -v $(pwd)/happy-sorter/config:/config \
  -v /path/to/sorted:/sorted \
  -v /path/to/download:/download \
  -v /path/to/TBC:/TBC \
  ghcr.io/testingbuddies24/happy-sorter:latest
```

Then open `http://localhost:8080` and walk through the setup wizard.

See [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md) for the full guide (docker-compose, NAS-specific notes, hardening, backup).

## Documentation

| Document | Purpose |
|---|---|
| [`docs/SPEC.md`](docs/SPEC.md) | Product spec — goals, requirements, success criteria |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | Components, data model, scraping flow, config schema |
| [`docs/ROADMAP.md`](docs/ROADMAP.md) | Build plan — milestone-by-milestone vertical slices |
| [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md) | Docker / docker-compose / NAS-specific deployment |
| [`docs/research/website-analysis.md`](docs/research/website-analysis.md) | What the original  actually is |
| [`docs/research/jav-metadata-standards.md`](docs/research/jav-metadata-standards.md) | JAV code format, studios, fields |
| [`docs/research/existing-projects.md`](docs/research/existing-projects.md) | OSS landscape — what already exists and why none of it "won" |
| [`docs/research/stack-recommendations.md`](docs/research/stack-recommendations.md) | Why Go + SQLite + HTMX |
| [`docs/research/source-test-results.md`](docs/research/source-test-results.md) | Live source probing — Cloudflare findings, source priority, mitigations |

## Update log

- **v1.0.10** (2026-08-19) — App icon redesigned to a clean kawaii smirk face on the brand indigo gradient tile (matches the dashboard's `--accent`). Dropped the busy funnel-with-heart that clashed with the UI palette.
- **v1.0.9** (2026-08-19) — FC2 lookups fixed: `fc2.javbus.com` is DNS-dead and JavBus dropped FC2 entirely, so FC2 now resolves through two new sources — **missav** (title, date, studio, genres, cover) and **javmenu** — plus JavDB's search card (its FC2 detail pages went login-only; queries now normalise `FC2-PPV-N` → `FC2-N`). Existing installs gain the new sources automatically on upgrade. Also: app icon (funnel + heart) across dashboard/README, modernised UI (stat tiles with TBC links, dark mode, version + repo footer), and the TBC page's noisy Filtered section moved to the bottom.
- **v1.0.8** (2026-08-17) — Code extractor rewritten from production log analysis: tolerates filename noise (site watermarks, `_CH`/`_4K`/`-uncensored` markers), multi-part releases (`-1`/`-2` share one folder), FC2-PPV codes, and falls back to the parent folder's code. Fixed SQLite `SQLITE_BUSY` write contention that silently lost "processed" records; hardened the watcher for torrent-completion bursts.
- **v1.0.7** (2026-07-27) — Field-level metadata merging across sources (plot, genres, series, rating, label fill each other's gaps); metadata and cover-image caching; dashboard live-activity backfill; TBC "refresh from disk"; bulk junk delete.
- **v1.0.6** (2026-07-27) — Folders renamed to `download`/`sorted`/`TBC`; live dashboard; poster cropped out of the wraparound package scan; folder-rename fallout repairs (config migration, safer TBC actions).
- **v1.0.5** (2026-07-20) — Real build version injected into the GUI; `proxy_url` actually wired into the scraper HTTP client.
- **v1.0.4** (2026-07-20) — Robust code extraction, enriched NFO, GUI redesign, folder-level dedup, duplicate routing to `TBC/_duplicate`.
- **v1.0.3** (2026-07-19) — Settle gate: never touch a file that is still being copied.
- **v1.0.2** (2026-07-19) — Watcher always polls (no missed events); `/setup/folders` read-only.
- **v1.0.1** (2026-07-18) — Release build fix (Dockerfile/go.mod Go version mismatch).
- **v1.0.0** (2026-07-18) — First release: watcher → rubbish filter → multi-source scrape (S1, IdeaPocket, JavBus, JavDB) → Jellyfin folder layout, setup GUI, TBC review queue, Docker image.

**Known gaps:** JavLibrary adapter deferred behind a Cloudflare challenge; per-actress photos not yet implemented.

For a hands-on sandbox to run the server yourself and drop test files in, see [`testbed/README.md`](testbed/README.md).

## Repository layout

```
HappySorter/
├── README.md                       ← this file
├── go.mod / go.sum
├── Dockerfile
├── docker-compose.yml
├── cmd/
│   └── server/                     # main entrypoint
├── internal/
│   ├── config/                     # config.yaml load/save + defaults
│   ├── database/                   # SQLite open + embedded migrations
│   ├── fsutil/                     # cross-device-safe file move helpers
│   ├── httpserver/                 # dashboard + /healthz
│   ├── logging/                    # slog JSON (stdout + logs table)
│   ├── nfo/                        # Kodi movie.nfo writer
│   ├── organiser/                  # Jellyfin folder layout + image download
│   ├── pipeline/                   # watcher -> filter -> scrape -> organise
│   ├── scraper/                    # Adapter interface, merging manager, s1/ideapocket/javbus/javdb adapters
│   ├── store/                      # files + metadata_cache tables
│   └── watcher/                    # /download folder watcher
├── web/                            # (future) HTMX templates + static
├── docs/
│   ├── SPEC.md
│   ├── ARCHITECTURE.md
│   ├── ROADMAP.md
│   ├── DEPLOYMENT.md
│   ├── index.md
│   └── research/
│       ├── website-analysis.md
│       ├── jav-metadata-standards.md
│       ├── existing-projects.md
│       ├── stack-recommendations.md
│       └── source-test-results.md
```

## Legal

HappySorter is a **personal-use tool**. It does not host, index, or redistribute scraped content. You are responsible for ensuring you have the right to possess the files you organise.

Scrape sources ship **disabled** by default — you opt in via the GUI. Scraping is done at low rates (default 1 QPS) and you agree to abide by each source's terms of service.

## License

[MIT](LICENSE) — free to use, modify, and self-host. Provided as-is, with no
warranty; use at your own risk (see [Legal](#legal) above).